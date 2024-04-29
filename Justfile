set export

ansible_default_opt := ""

default:
  @just --list

setup:
  @if ! {{path_exists("ansible/group_vars")}}; then \
    cd ansible && ln -s ../private/ansible/group_vars group_vars; \
  fi

auth:
  ssh -T -F ansible/ssh_config ansible_user@arm-srv.shiron.dev

ansible opt=ansible_default_opt: setup
  cd ansible && ansible-playbook -i hosts.yml site.yml -C {{opt}}

ansible-run opt=ansible_default_opt: setup
  cd ansible && ansible-playbook -i hosts.yml site.yml {{opt}}

private-gen:
  cd private && just gen
