variable "lambda_function_name" {
  type = string
}

variable "lambda_function_invoke_arn" {
  type = string
}

variable "route53_hostzone_name" {
  type = string
}

variable "api_gateway_domain" {
  type = string
}

variable "acm_certificate_domain" {
  type = string
}
