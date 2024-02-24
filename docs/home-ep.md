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
