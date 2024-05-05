set export

ansible_default_opt := ""

default:
  @just --list

setup:
  @if ! {{path_exists("ansible/group_vars")}}; then \
    cd ansible && ln -s ../private/ansible/group_vars group_vars; \
  fi
  @if ! {{path_exists("ansible/roles/asterisk/templates")}}; then \
    cd ansible/roles/asterisk && ln -s ../../../private/ansible/roles/asterisk/templates templates; \
  fi
  @if ! {{path_exists("ansible/roles/fail2ban/templates")}}; then \
    cd ansible/roles/fail2ban && ln -s ../../../private/ansible/roles/fail2ban/templates templates; \
  fi

auth:
  ssh -T -F ansible/ssh_config ansible_user@arm-srv.shiron.dev

ansible opt=ansible_default_opt: setup
  cd ansible && ansible-playbook -i hosts.yml site.yml -C {{opt}}

ansible-run opt=ansible_default_opt: setup
  cd ansible && ansible-playbook -i hosts.yml site.yml {{opt}}

private-gen:
  cd private && just gen
