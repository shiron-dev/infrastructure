locals {
  cloudflare_account_id = "edc628145468437b85dc0e6d48bff3e3"
  arm_srv_tunnel_id     = "6b6ae57d-ce74-4db1-b122-63f23053ec1f"
  arm_srv_app_id        = "4156d5c6-ec06-4ec4-89b0-998066f0f175"

  github_actions_policy_name = "Allow GitHub Actions Service Token"

  existing_policy_refs = [
    for p in data.cloudflare_zero_trust_access_application.arm_srv.policies : {
      id         = p.id
      precedence = p.precedence
    } if p.name != local.github_actions_policy_name
  ]

  existing_policy_precedences = [
    for p in local.existing_policy_refs : p.precedence
  ]

  github_actions_policy_precedence = (
    length(local.existing_policy_precedences) > 0 ?
    max(local.existing_policy_precedences...) + 1 :
    1
  )
}

data "cloudflare_zero_trust_access_application" "arm_srv" {
  account_id = local.cloudflare_account_id
  app_id     = local.arm_srv_app_id
}

resource "cloudflare_zero_trust_tunnel_cloudflared" "arm_srv" {
  account_id = local.cloudflare_account_id
  name       = "oci-arm"
  config_src = "cloudflare"
}

resource "cloudflare_zero_trust_access_service_token" "github_actions_arm_srv" {
  account_id = local.cloudflare_account_id
  name       = "github-actions-arm-srv-ssh"
  duration   = "8760h"
}

resource "cloudflare_zero_trust_access_application" "arm_srv" {
  account_id                  = local.cloudflare_account_id
  name                        = data.cloudflare_zero_trust_access_application.arm_srv.name
  domain                      = data.cloudflare_zero_trust_access_application.arm_srv.domain
  type                        = data.cloudflare_zero_trust_access_application.arm_srv.type
  session_duration            = data.cloudflare_zero_trust_access_application.arm_srv.session_duration
  service_auth_401_redirect   = true
  auto_redirect_to_identity   = data.cloudflare_zero_trust_access_application.arm_srv.auto_redirect_to_identity
  app_launcher_visible        = data.cloudflare_zero_trust_access_application.arm_srv.app_launcher_visible
  allow_authenticate_via_warp = data.cloudflare_zero_trust_access_application.arm_srv.allow_authenticate_via_warp

  policies = concat(
    local.existing_policy_refs,
    [
      {
        name       = local.github_actions_policy_name
        decision   = "non_identity"
        precedence = local.github_actions_policy_precedence
        include = [
          {
            service_token = {
              token_id = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.id
            }
          }
        ]
      }
    ]
  )
}

import {
  to = cloudflare_zero_trust_tunnel_cloudflared.arm_srv
  id = "${local.cloudflare_account_id}/${local.arm_srv_tunnel_id}"
}

import {
  to = cloudflare_zero_trust_access_application.arm_srv
  id = "accounts/${local.cloudflare_account_id}/${local.arm_srv_app_id}"
}

output "github_actions_arm_srv_service_token_client_id" {
  description = "Cloudflare Access service token client ID for GitHub Actions"
  value       = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_id
}

output "github_actions_arm_srv_service_token_client_secret" {
  description = "Cloudflare Access service token client secret for GitHub Actions"
  value       = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_secret
  sensitive   = true
}

resource "github_actions_environment_secret" "cloudflare_access_client_id" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "CF_ACCESS_CLIENT_ID"
  plaintext_value = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_id
}

resource "github_actions_environment_secret" "cloudflare_access_client_secret" {
  for_each = local.github_environments

  repository      = local.github_repository
  environment     = each.value
  secret_name     = "CF_ACCESS_CLIENT_SECRET"
  plaintext_value = cloudflare_zero_trust_access_service_token.github_actions_arm_srv.client_secret
}
