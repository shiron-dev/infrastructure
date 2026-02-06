resource "google_project_service" "cloudkms_api" {
  service = "cloudkms.googleapis.com"
}
