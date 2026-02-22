locals {
  api_cloudkms         = "cloudkms.googleapis.com"
  api_iamcredentials   = "iamcredentials.googleapis.com"
  api_sts              = "sts.googleapis.com"
  project_services     = toset([local.api_cloudkms, local.api_iamcredentials, local.api_sts])
}

resource "google_project_service" "apis" {
  for_each = local.project_services
  service  = each.key
}
