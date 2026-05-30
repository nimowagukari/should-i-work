module "lambda" {
  source = "./modules/lambda"
}

module "apigateway" {
  source = "./modules/apigateway"

  lambda_function_name       = module.lambda.lambda_function_name
  lambda_function_invoke_arn = module.lambda.lambda_function_invoke_arn
  route53_hostzone_name      = var.route53_hostzone_name
  api_gateway_domain         = var.api_gateway_domain
  acm_certificate_domain     = var.acm_certificate_domain
}
