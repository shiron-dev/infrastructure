ANSIBLE_DEFAULT_OPT ?=
ANSIBLE_DIR := ansible
UV_ANSIBLE := uv run --project tools/ansible --
PROJECT_ID := shiron-dev

CHECK_SECRETS_SCRIPT := scripts/check-secrets.sh

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
	@echo "Initializing..."

.PHONY: ansible-init
ansible-init: init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-galaxy install -r requirements.yml"

.PHONY: ansible-lint
ansible-lint: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-lint -c .ansible-lint --fix"

define check_gcloud_auth
	@if ! (gcloud config get-value project 2>/dev/null | grep -q "^$(PROJECT_ID)$$" && \
	    gcloud auth list --filter=status:ACTIVE --format="value(account)" | grep -q . && \
	    gcloud auth application-default print-access-token >/dev/null 2>&1); then \
		gcloud auth login; \
		gcloud auth application-default login; \
	fi
endef

.PHONY: auth
auth: init
	$(call check_gcloud_auth)
	gcloud config set project $(PROJECT_ID)
	ssh -o BatchMode=yes -o ConnectTimeout=5 -F $(ANSIBLE_DIR)/ssh_config ansible_user@arm-srv.shiron.dev exit

.PHONY: ansible-ci
ansible-ci: ansible-lint

.PHONY: ansible-check
ansible-check: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml -C $(ANSIBLE_DEFAULT_OPT)"

.PHONY: ansible-run
ansible-run: ansible-init
	$(UV_ANSIBLE) bash -c "cd $(ANSIBLE_DIR) && ansible-playbook -i hosts.yml site.yml $(ANSIBLE_DEFAULT_OPT)"

.PHONY: terraform-init
terraform-init: init
	cd terraform && terraform init

.PHONY: terraform-plan
terraform-plan: terraform-init
	cd terraform && terraform plan

.PHONY: terraform-apply
terraform-apply: terraform-init
	cd terraform && terraform apply

.PHONY: terraform-lint
terraform-lint: terraform-init
	cd terraform && tflint

.PHONY: terraform-fmt
terraform-fmt:
	cd terraform && terraform fmt -recursive

.PHONY: terraform-validate
terraform-validate: terraform-init
	cd terraform && terraform validate

# https://github.com/aquasecurity/trivy - IaC misconfig and vulnerability scanning
.PHONY: terraform-trivy
terraform-trivy:
	trivy config terraform/

.PHONY: terraform-ci
terraform-ci: terraform-lint terraform-fmt terraform-validate terraform-trivy

# コスト比較前のベースライン作成
.PHONY: infracost-base
infracost-base: terraform-plan
	cd terraform && infracost breakdown --path=. --format json --out-file infracost-base.json

# コスト比較
.PHONY: infracost-diff
infracost-diff: terraform-plan
	@if [ ! -f terraform/infracost-base.json ]; then \
		echo "Error: infracost-base.json not found. Run 'make infracost-base' first."; \
		exit 1; \
	fi
	cd terraform && infracost diff --path=. --compare-to infracost-base.json

# 今のコストチェック
.PHONY: infracost-breakdown
infracost-breakdown: terraform-plan
	cd terraform && infracost breakdown --path=.

.PHONY: sops-encrypt
sops-encrypt:
	@echo "Encrypting with SOPS..."; \
	if [ -n "$(FILE)" ]; then \
		if [ -f "$(FILE)" ] && [ "$${FILE##*.}" != "sops" ]; then FILES="$(FILE)"; \
		elif [ -f "$(FILE)" ] && [ "$${FILE##*.}" = "sops" ]; then base="$${FILE%.sops}"; if [ -f "$$base" ]; then FILES="$$base"; else echo "Error: plaintext $$base not found for $(FILE)" >&2; exit 1; fi; \
		elif [ -f "$(FILE).sops" ]; then base="$(FILE)"; if [ -f "$$base" ]; then FILES="$$base"; else echo "Error: plaintext $$base not found (got $(FILE).sops)" >&2; exit 1; fi; \
		else echo "Error: $(FILE) not found" >&2; exit 1; fi; \
	else \
		FILES="$$(find . -name "*.secrets.*" -type f ! -name "*.sops")"; \
	fi; \
	for file in $$FILES; do \
		echo "Encrypting $$file..."; \
		sops --output-type json --encrypt "$$file" > "$$file.sops"; \
		chmod -w "$$file" "$$file.sops"; \
	done

.PHONY: sops-decrypt
sops-decrypt:
	@echo "Decrypting with SOPS..."; \
	if [ -n "$(FILE)" ]; then \
		if [ -f "$(FILE)" ]; then FILES="$(FILE)"; \
		elif [ -f "$(FILE).sops" ]; then FILES="$(FILE).sops"; \
		else echo "Error: $(FILE) or $(FILE).sops not found" >&2; exit 1; fi; \
	else \
		FILES="$$(find . -name "*.secrets.*.sops" -type f)"; \
	fi; \
	for file in $$FILES; do \
		echo "Decrypting $$file..."; \
		base="$${file%.sops}"; \
		ext="$${base##*.}"; \
		case "$$ext" in \
		  yaml|yml) output_type="yaml" ;; \
		  *) output_type="binary" ;; \
		esac; \
		if [ -f "$$base" ]; then chmod +w "$$base"; fi; \
		sops --decrypt --output-type "$$output_type" "$$file" > "$$base"; \
		chmod -w "$$base" "$$file"; \
	done

.PHONY: sops-edit
sops-edit:
	@$(MAKE) sops-decrypt $(if $(FILE),FILE="$(FILE)",); \
	if [ -n "$(FILE)" ]; then \
		base="$(FILE)"; \
		[ "$${base%.sops}" != "$$base" ] && base="$${base%.sops}"; \
		chmod +w "$$base" "$$base.sops"; \
		echo "Edit the decrypted file(s). Press Enter when done to re-encrypt."; \
		code --wait "$$base"; \
		$(MAKE) sops-encrypt FILE="$$base"; \
	else \
		for f in $$(find . -name "*.secrets.*.sops" -type f); do \
			chmod +w "$${f%.sops}" "$$f"; \
		done; \
		echo "Edit the decrypted file(s). Press Enter when done to re-encrypt."; \
		read -r _; \
		$(MAKE) sops-encrypt; \
	fi

.PHONY: sops-ci
sops-ci:
	@echo "Checking for unencrypted secrets tracked by git..."; \
	FILES="$$(find . -name '*.secrets.*' ! -name '*.secrets.*.sops' -type f)"; \
	EXIT=0; \
	for file in $$FILES; do \
		if git ls-files --error-unmatch "$$file" >/dev/null 2>&1; then \
			echo "Error: Unencrypted secrets file tracked by git: $$file" >&2; \
			EXIT=1; \
		fi; \
	done; \
	if [ $$EXIT -ne 0 ]; then \
		echo "One or more unencrypted secrets files are tracked by git. Please remove them from version control." >&2; \
		exit 1; \
	fi

.PHONY: kics
kics:
	@dir=$$(mktemp -d) && \
	rsync -a --exclude='tools/ansible/.venv' $(PWD)/ $$dir/ && \
	docker run -t -v $$dir:/path checkmarx/kics scan -p /path; \
	status=$$?; rm -rf $$dir; exit $$status

.PHONY: ci
ci:
	@if git diff --name-only origin/main...HEAD | grep -q "^ansible/"; then \
		echo "Running ansible-ci..."; \
		$(MAKE) ansible-ci; \
	fi
	@if git diff --name-only origin/main...HEAD | grep -q "^terraform/"; then \
		echo "Running terraform-ci..."; \
		$(MAKE) terraform-ci; \
	fi

	echo "Running sops-ci...";
	$(MAKE) sops-ci;

	echo "Running kics...";
	$(MAKE) kics;

.DEFAULT_GOAL := help
