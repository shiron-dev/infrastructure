terraform {
  required_providers {
    cloudflare = {
      source = "cloudflare/cloudflare"
    }
    google = {
      source  = "hashicorp/google"
      version = "7.20.0"
    }
    github = {
      source  = "integrations/github"
      version = "6.11.1"
    }
  }

  required_version = ">= 1.14.5"

  backend "gcs" {
    bucket = "shiron-dev-terraform"
    prefix = "terraform/state"
  }
}

provider "google" {
  project = "shiron-dev"
  region  = "asia-northeast1"
}

provider "github" {
  owner = local.github_owner
}

provider "cloudflare" {
  api_token = var.cloudflare_api_token
}
