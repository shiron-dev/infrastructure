ANSIBLE_DEFAULT_OPT ?=
ANSIBLE_DIR := ansible

.PHONY: help
help:
	@echo "Usage: make [target]"
	@echo ""
	@echo "Available targets:"
	@awk ' BEGIN { FS = ":[ \t]*"; comment = "" } /^#/ { comment = substr($$0, 3); next } /^\.PHONY:/ { if ($$2) { \
			n = split($$2, targets, " "); for (i = 1; i <= n; i++) {if (targets[i] != "") {printf "  \033[36m%-20s\033[0m %s\n", targets[i], comment;}}} comment = "";} \
		{ if (!/^\.PHONY:/) { comment = "" } }' $(MAKEFILE_LIST)

.PHONY: init
init:
	cd $(ANSIBLE_DIR) && ansible-galaxy install -r requirements.yml

.PHONY: lint
lint: init
	cd $(ANSIBLE_DIR) && ansible-lint

.PHONY: lint-fix
lint-fix: init
	cd $(ANSIBLE_DIR) && ansible-lint --fix

.PHONY: auth
auth: init
	ssh -T -F $(ANSIBLE_DIR)/ssh_config ansible_user@arm-srv.shiron.dev

.PHONY: ansible-check
ansible-check: init
	cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml -C $(ANSIBLE_DEFAULT_OPT)

.PHONY: ansible-run
ansible-run: init
	cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml $(ANSIBLE_DEFAULT_OPT)


.DEFAULT_GOAL := help
