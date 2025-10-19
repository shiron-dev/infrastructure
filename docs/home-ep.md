# home-ep

お家環境と外とをつなぐ入り口

## 準備

### hostname の変更

```sh
sudo hostnamectl set-hostname home-ep
```

### TailScale のインストール

```sh
curl -fsSL https://tailscale.com/install.sh | sh
sudo tailscale up --ssh --advertise-exit-node
```

```/etc/sysctl.conf
net.ipv4.ip_forward=1
net.ipv6.conf.all.forwarding=1
```

```sh
sudo sysctl -p /etc/sysctl.conf
```

### Ansible ユーザーの作成

※CI/CD の利便性のために NOPASSWD を指定しているので、注入後は直ちにユーザーを削除するか、パスワードを変更すること

```sh
sudo su -
adduser ansible_user
usermod -aG sudo ansible_user
echo "ansible_user ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers.d/ansible_user
```

## Homer + Cloudflare Tunnel

Homer はシンプルな静的ダッシュボードです（参考: https://github.com/bastienwirtz/homer）。`home_ep` では Docker Compose で `homer` と `cloudflared` を起動します。

### シークレット設定

`ansible/group_vars/home_ep/homer.secrets.yml` に Cloudflare Tunnel のトークンを設定し、SOPS で暗号化してください。

```yaml
cf_tunnel_token: "<your-tunnel-token>"
```

### デプロイ

タグを使って `homer` のみ実行可能です。

```sh
make ansible-run ANSIBLE_DEFAULT_OPT="--tags homer"
```

デフォルトでは Homer はホストのポート `8080` で起動し、Cloudflare Tunnel 経由で外部公開されます。
