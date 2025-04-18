aws_region        = "us-east-1"
project_name      = "ollystack"
instance_type     = "t3.large"
spot_max_price    = "0.07"
root_volume_size  = 30
data_volume_size  = 30
existing_key_pair = "mb2025"
preferred_az      = "us-east-1c"

ssh_allowed_cidr  = ["0.0.0.0/0"]
app_allowed_cidr  = ["0.0.0.0/0"]
otlp_allowed_cidr = ["0.0.0.0/0"]

clickhouse_password = "changeme"
jwt_secret          = "mvp-jwt-secret-change-in-prod"
