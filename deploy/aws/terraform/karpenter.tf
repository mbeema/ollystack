# Karpenter Configuration for Cost-Optimized Node Provisioning

# =============================================================================
# Kubernetes Provider (after EKS is created)
# =============================================================================

provider "kubernetes" {
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "/usr/local/Cellar/awscli/2.22.20/bin/aws"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
  }
}

provider "helm" {
  kubernetes {
    host                   = module.eks.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)

    exec {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "/usr/local/Cellar/awscli/2.22.20/bin/aws"
      args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
    }
  }
}

provider "kubectl" {
  apply_retry_count      = 5
  host                   = module.eks.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks.cluster_certificate_authority_data)
  load_config_file       = false

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "/usr/local/Cellar/awscli/2.22.20/bin/aws"
    args        = ["eks", "get-token", "--cluster-name", module.eks.cluster_name]
  }
}

# =============================================================================
# Karpenter Helm Release
# =============================================================================

resource "helm_release" "karpenter" {
  namespace        = "karpenter"
  create_namespace = true
  name             = "karpenter"
  repository       = "oci://public.ecr.aws/karpenter"
  chart            = "karpenter"
  version          = "1.0.0"

  values = [
    <<-EOT
    settings:
      clusterName: ${module.eks.cluster_name}
      clusterEndpoint: ${module.eks.cluster_endpoint}
      interruptionQueue: ${module.karpenter.queue_name}
    serviceAccount:
      annotations:
        eks.amazonaws.com/role-arn: ${module.karpenter.iam_role_arn}
    controller:
      resources:
        requests:
          cpu: 100m
          memory: 256Mi
        limits:
          cpu: 500m
          memory: 512Mi
    EOT
  ]

  depends_on = [module.eks, module.karpenter]
}

# =============================================================================
# Karpenter EC2NodeClass
# =============================================================================

resource "kubectl_manifest" "karpenter_node_class" {
  yaml_body = <<-YAML
    apiVersion: karpenter.k8s.aws/v1
    kind: EC2NodeClass
    metadata:
      name: default
    spec:
      amiFamily: AL2
      role: ${module.karpenter.node_iam_role_name}
      subnetSelectorTerms:
        - tags:
            karpenter.sh/discovery: ${var.cluster_name}
      securityGroupSelectorTerms:
        - tags:
            karpenter.sh/discovery: ${var.cluster_name}
      tags:
        karpenter.sh/discovery: ${var.cluster_name}
        Environment: ${var.environment}
      blockDeviceMappings:
        - deviceName: /dev/xvda
          ebs:
            volumeSize: 50Gi
            volumeType: gp3
            encrypted: true
            deleteOnTermination: true
  YAML

  depends_on = [helm_release.karpenter]
}

# =============================================================================
# NodePool: Spot ARM64 (for stateless workloads - maximum savings)
# =============================================================================

resource "kubectl_manifest" "karpenter_nodepool_spot_arm64" {
  yaml_body = <<-YAML
    apiVersion: karpenter.sh/v1
    kind: NodePool
    metadata:
      name: spot-arm64
    spec:
      template:
        metadata:
          labels:
            capacity-type: spot
            arch: arm64
            workload-type: stateless
        spec:
          requirements:
            - key: kubernetes.io/arch
              operator: In
              values: ["arm64"]
            - key: karpenter.sh/capacity-type
              operator: In
              values: ["spot"]
            - key: node.kubernetes.io/instance-type
              operator: In
              values:
                - t4g.small
                - t4g.medium
                - t4g.large
                - m7g.medium
                - m7g.large
                - c7g.medium
                - c7g.large
          nodeClassRef:
            group: karpenter.k8s.aws
            kind: EC2NodeClass
            name: default
          expireAfter: 720h  # 30 days
      limits:
        cpu: 100
        memory: 200Gi
      disruption:
        consolidationPolicy: WhenEmptyOrUnderutilized
        consolidateAfter: 1m
      weight: 100  # Highest priority - try Spot first
  YAML

  depends_on = [kubectl_manifest.karpenter_node_class]
}

# =============================================================================
# NodePool: On-Demand ARM64 (for stateful workloads - ClickHouse, Redis)
# =============================================================================

resource "kubectl_manifest" "karpenter_nodepool_ondemand_arm64" {
  yaml_body = <<-YAML
    apiVersion: karpenter.sh/v1
    kind: NodePool
    metadata:
      name: ondemand-arm64
    spec:
      template:
        metadata:
          labels:
            capacity-type: on-demand
            arch: arm64
            workload-type: stateful
        spec:
          requirements:
            - key: kubernetes.io/arch
              operator: In
              values: ["arm64"]
            - key: karpenter.sh/capacity-type
              operator: In
              values: ["on-demand"]
            - key: node.kubernetes.io/instance-type
              operator: In
              values:
                - r7g.large    # 2 vCPU, 16GB - ClickHouse small
                - r7g.xlarge   # 4 vCPU, 32GB - ClickHouse medium
                - r7g.2xlarge  # 8 vCPU, 64GB - ClickHouse large
                - m7g.large
                - m7g.xlarge
          nodeClassRef:
            group: karpenter.k8s.aws
            kind: EC2NodeClass
            name: default
          expireAfter: 720h
      limits:
        cpu: 50
        memory: 400Gi
      disruption:
        consolidationPolicy: WhenEmpty
        consolidateAfter: 10m
      weight: 10  # Lower priority - use after Spot
  YAML

  depends_on = [kubectl_manifest.karpenter_node_class]
}

# =============================================================================
# NodePool: On-Demand x86 (fallback if ARM not available)
# =============================================================================

resource "kubectl_manifest" "karpenter_nodepool_fallback_x86" {
  yaml_body = <<-YAML
    apiVersion: karpenter.sh/v1
    kind: NodePool
    metadata:
      name: fallback-x86
    spec:
      template:
        metadata:
          labels:
            capacity-type: on-demand
            arch: amd64
            workload-type: fallback
        spec:
          requirements:
            - key: kubernetes.io/arch
              operator: In
              values: ["amd64"]
            - key: karpenter.sh/capacity-type
              operator: In
              values: ["on-demand", "spot"]
            - key: node.kubernetes.io/instance-type
              operator: In
              values:
                - t3.medium
                - t3.large
                - m6i.large
                - m6i.xlarge
          nodeClassRef:
            group: karpenter.k8s.aws
            kind: EC2NodeClass
            name: default
          expireAfter: 720h
      limits:
        cpu: 20
        memory: 80Gi
      disruption:
        consolidationPolicy: WhenEmptyOrUnderutilized
        consolidateAfter: 5m
      weight: 1  # Lowest priority - last resort
  YAML

  depends_on = [kubectl_manifest.karpenter_node_class]
}

# Note: kubectl provider is defined in main.tf
