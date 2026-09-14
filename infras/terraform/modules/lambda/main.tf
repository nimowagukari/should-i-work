locals {
  app_identifier = "should-i-work"
}

data "aws_caller_identity" "current" {}
data "aws_region" "current" {}

resource "aws_lambda_function" "this" {
  function_name = local.app_identifier
  role          = aws_iam_role.this.arn

  handler = "bootstrap"
  runtime = "provided.al2023"

  package_type     = "Zip"
  filename         = data.archive_file.this.output_path
  source_code_hash = data.archive_file.this.output_base64sha256

  logging_config {
    log_format = "JSON"
    log_group  = aws_cloudwatch_log_group.this.name
  }

  # 初回作成後のコード更新は `aws lambda update-function-code` で行うため、
  # terraform apply では関数コードの差分を無視する
  lifecycle {
    ignore_changes = [filename, source_code_hash]
  }
}
resource "aws_iam_role" "this" {
  name = "${local.app_identifier}-lambda-role"
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          Service = "lambda.amazonaws.com"
        }
      }
    ]
  })
}
resource "aws_iam_role_policy" "cloudwatch_logs" {
  name = "CloudwatchLogs"
  role = aws_iam_role.this.id
  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = [
          "logs:CreateLogStream",
          "logs:PutLogEvents"
        ]
        Effect   = "Allow"
        Resource = "arn:aws:logs:${data.aws_region.current.region}:${data.aws_caller_identity.current.account_id}:log-group:/aws/lambda/${local.app_identifier}:*"
      }
    ]
  })
}
resource "aws_cloudwatch_log_group" "this" {
  name              = "/aws/lambda/${local.app_identifier}"
  retention_in_days = 7
}
data "archive_file" "this" {
  type        = "zip"
  output_path = "${path.module}/lambda.zip"
  source_file = "/workspaces/should-i-work/bootstrap"
}
