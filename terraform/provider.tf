terraform {
  required_providers {
    google = {
      source  = "hashicorp/google"
      version = "7.7.0"
    }
    local = {
      source  = "hashicorp/local"
      version = ">= 2.5.1"
    }
    cloudflare = {
      source  = "cloudflare/cloudflare"
      version = "= 5.11.0"
    }
  }

  required_version = ">= 1.13.1"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state"
  }
}

provider "google" {
  project = "shiron-dev"
  region  = "asia-northeast1"
}

variable "cloudflare_api_token" {
  description = "Cloudflare API Token"
  type        = string
  sensitive   = true
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}


