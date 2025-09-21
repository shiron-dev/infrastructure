resource "google_kms_key_ring" "sops" {
  name     = "sops"
  location = "global"

  depends_on = [google_project_service.cloudkms_api]
}

resource "google_kms_crypto_key" "sops_key" {
  name     = "sops-key"
  key_ring = google_kms_key_ring.sops.id
  purpose  = "ENCRYPT_DECRYPT"
}

output "kms_keyring_name" {
  value = google_kms_key_ring.sops.name
}

output "kms_key_name" {
  value = google_kms_crypto_key.sops_key.name
}

output "kms_key_id" {
  value = google_kms_crypto_key.sops_key.id
}
