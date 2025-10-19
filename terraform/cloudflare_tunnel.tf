resource "cloudflare_zero_trust_tunnel_cloudflared" "arm_service_honor" {
  account_id = local.cloudflare_account_id
  name       = "arm-service-honor"
}

resource "cloudflare_zero_trust_tunnel_cloudflared_config" "arm_service_honor_config" {
  account_id = local.cloudflare_account_id
  tunnel_id  = cloudflare_zero_trust_tunnel_cloudflared.arm_service_honor.id

  config = {
    ingress = [
      {
        hostname = "services.shiron.dev"
        service  = "http://homer:8080"
      },
      {
        service = "http_status:404"
      }
    ]
  }
}

resource "cloudflare_dns_record" "arm_service_honor_cname" {
  zone_id = local.cloudflare_shiron_dev_zone_id
  name    = "services.shiron.dev"
  type    = "CNAME"
  content = "${cloudflare_zero_trust_tunnel_cloudflared.arm_service_honor.id}.cfargotunnel.com"
  proxied = true
  ttl     = 1
}
