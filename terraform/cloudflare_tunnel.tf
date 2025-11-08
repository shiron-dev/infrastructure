resource "cloudflare_zero_trust_tunnel_cloudflared" "arm_service_honor" {
  account_id = local.cloudflare_account_id
  name       = "arm-service-honor"
}


resource "cloudflare_dns_record" "arm_service_honor_cname" {
  zone_id = local.cloudflare_shiron_dev_zone_id
  name    = "services.shiron.dev"
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.arm_service_honor.id}.cfargotunnel.com"
  proxied = true
  ttl     = 1
}

output "arm_service_honor_tunnel_token" {
  value = cloudflare_zero_trust_tunnel_cloudflared.arm_service_honor.id
  sensitive = true
}
