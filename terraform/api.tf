resource "google_project_service" "cloudkms_api" {
  service            = "cloudkms.googleapis.com"
  disable_on_destroy = false
}
