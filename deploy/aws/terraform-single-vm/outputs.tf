# =============================================================================
# Outputs
# =============================================================================

output "instance_id" {
  description = "EC2 instance ID"
  value       = aws_instance.ollystack.id
}

output "public_ip" {
  description = "Elastic IP address (stable across spot interruptions)"
  value       = aws_eip.ollystack.public_ip
}

output "private_ip" {
  description = "Private IP address"
  value       = aws_instance.ollystack.private_ip
}

output "ssh_command" {
  description = "SSH command to connect to the instance"
  value       = "ssh -i <your-key.pem> ec2-user@${aws_eip.ollystack.public_ip}"
}

output "web_ui_url" {
  description = "Web UI URL"
  value       = "http://${aws_eip.ollystack.public_ip}:3000"
}

output "api_url" {
  description = "API Server URL"
  value       = "http://${aws_eip.ollystack.public_ip}:8080"
}

output "otlp_grpc_endpoint" {
  description = "OTLP gRPC endpoint for sending telemetry"
  value       = "${aws_eip.ollystack.public_ip}:4317"
}

output "otlp_http_endpoint" {
  description = "OTLP HTTP endpoint for sending telemetry"
  value       = "http://${aws_eip.ollystack.public_ip}:4318"
}

output "spot_price_info" {
  description = "Note about spot pricing"
  value       = "Using spot instance - estimated cost ~$10-15/month"
}
