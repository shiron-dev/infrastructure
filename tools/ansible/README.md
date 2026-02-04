# Ansible tools

UV-managed environment for [Ansible](https://docs.ansible.com/) and [ansible-lint](https://docs.ansible.com/projects/lint/) used by CI and optionally for local runs.

## Usage

From repo root:

```bash
cd tools/ansible && uv sync
cd ../../ansible && ansible-galaxy install -r requirements.yml && ansible-lint
```

Or run via the project venv:

```bash
uv run --project tools/ansible ansible-lint ansible/
```
