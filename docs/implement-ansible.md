# Ansible-Based Kubernetes Node Management

Automate the provisioning and configuration of Raspberry Pi K3s nodes using Ansible, with multi-user support, SSH key authentication, and Docker-based testing.

## Background

Currently, node setup is done via [install.sh](../scripts/raspberrypi-k3s/remote/install.sh), which handles:

- Raspberry Pi cgroup configuration
- Tailscale installation and authentication
- K3s installation (server/agent modes)
- Tailscale service host configuration

This plan introduces Ansible to manage user setup and provide a more declarative, multi-user workflow.

---

## Proposed Directory Structure

```
scripts/raspberrypi-k3s/
├── ansible/
│   ├── inventory/
│   │   └── hosts.yml              # Inventory file
│   ├── roles/
│   │   ├── user_setup/            # User creation + SSH keys
│   │   │   └── tasks/main.yml
│   │   ├── k3s_install/           # Wraps install.sh execution
│   │   │   └── tasks/main.yml
│   │   └── pi_prepare/            # Optional: cgroups, iptables
│   │       └── tasks/main.yml
│   ├── playbooks/
│   │   ├── site.yml               # Full provisioning playbook
│   │   └── user_setup.yml         # User-only playbook
│   ├── group_vars/
│   │   └── all.yml                # Common variables
│   └── ansible.cfg                # Ansible configuration
├── public_ssh_keys/               # EXISTING - SSH public keys
│   ├── tbigelow.pub
│   └── ddowell.pub
├── testing/
│   ├── Dockerfile                 # Test container with SSH
│   ├── docker-compose.yml         # Container orchestration
│   └── run_tests.sh               # Test runner script
└── remote/
    └── install.sh                 # EXISTING - K3s install script
```

---

## Proposed Changes

### Component 1: Ansible Core Infrastructure

Set up the base Ansible configuration and inventory structure.

#### [NEW] ansible/ansible.cfg

```ini
[defaults]
inventory = inventory/hosts.yml
roles_path = roles
host_key_checking = False
# Remote user will be detected (ddowell or tbigelow)

[privilege_escalation]
become = True
become_method = sudo
```

#### [NEW] ansible/inventory/hosts.yml

```yaml
all:
  children:
    k3s_servers:
      hosts:
        # Add server nodes here
        # server1:
        #   ansible_host: 100.x.y.z
    k3s_agents:
      hosts:
        # Add agent nodes here
```

#### [NEW] ansible/group_vars/all.yml

```yaml
# Users to configure on all nodes
managed_users:
  - name: ddowell
    pubkey_file: "{{ playbook_dir }}/../../public_ssh_keys/ddowell.pub"
  - name: tbigelow
    pubkey_file: "{{ playbook_dir }}/../../public_ssh_keys/tbigelow.pub"

# Detected running user (set dynamically)
local_user: "{{ lookup('env', 'USER') }}"

# Secrets from environment (loaded by env_loader.sh)
ts_authkey: "{{ lookup('env', 'TS_AUTHKEY') }}"
k3s_token: "{{ lookup('env', 'K3S_TOKEN') }}"
ts_service_name: "{{ lookup('env', 'TS_SERVICE_NAME') | default('svc:chalupa-k3s', true) }}"
```

---

### Component 2: User Detection & Setup Role

This role handles multi-user detection and provisioning with SSH keys.

#### [NEW] ansible/roles/user_setup/tasks/main.yml

```yaml
---
# Detect and validate the local running user
- name: Detect local running user
  set_fact:
    detected_user: "{{ lookup('env', 'USER') }}"
  delegate_to: localhost
  run_once: true

- name: Validate detected user is allowed
  assert:
    that:
      - detected_user in ['ddowell', 'tbigelow']
    fail_msg: "Running user '{{ detected_user }}' is not in the allowed list [ddowell, tbigelow]"
    success_msg: "Detected running user: {{ detected_user }}"
  delegate_to: localhost
  run_once: true

# Create managed users on target nodes
- name: Create managed users
  user:
    name: "{{ item.name }}"
    state: present
    shell: /bin/bash
    create_home: yes
    groups: sudo
    append: yes
  loop: "{{ managed_users }}"

# Deploy SSH authorized keys
- name: Get SSH public key content
  set_fact:
    user_pubkeys: >-
      {{
        user_pubkeys | default([]) + [
          {
            'name': item.name,
            'key': lookup('file', item.pubkey_file)
          }
        ]
      }}
  loop: "{{ managed_users }}"
  delegate_to: localhost
  run_once: true

- name: Deploy authorized_keys for users
  authorized_key:
    user: "{{ item.name }}"
    key: "{{ item.key }}"
    state: present
    exclusive: no
  loop: "{{ user_pubkeys }}"

# Configure passwordless sudo
- name: Configure passwordless sudo for managed users
  lineinfile:
    path: /etc/sudoers.d/{{ item.name }}
    line: "{{ item.name }} ALL=(ALL) NOPASSWD: ALL"
    create: yes
    mode: "0440"
    validate: "visudo -cf %s"
  loop: "{{ managed_users }}"
```

---

### Component 3: K3s Installation Role

Wraps the existing `install.sh` for Ansible execution.

#### [NEW] ansible/roles/k3s_install/tasks/main.yml

```yaml
---
# Copy install.sh to target node
- name: Copy K3s install script
  copy:
    src: "{{ playbook_dir }}/../remote/install.sh"
    dest: /tmp/install.sh
    mode: "0755"

# Execute install.sh with required environment
- name: Run K3s installation
  shell: /tmp/install.sh {{ k3s_role }} {{ k3s_server_ip | default('') }}
  environment:
    TS_AUTHKEY: "{{ ts_authkey }}"
    K3S_TOKEN: "{{ k3s_token | default('') }}"
    TS_SERVICE_NAME: "{{ ts_service_name | default('svc:chalupa-k3s') }}"
  args:
    executable: /bin/bash
  register: k3s_install_result

- name: Display K3s installation output
  debug:
    var: k3s_install_result.stdout_lines

# Cleanup
- name: Remove install script
  file:
    path: /tmp/install.sh
    state: absent
```

---

### Component 4: Pi Preparation Role (Optional)

Can extract cgroup/iptables checks from install.sh for earlier execution.

#### [NEW] ansible/roles/pi_prepare/tasks/main.yml

```yaml
---
# Check for cgroup memory settings (extracted from install.sh)
- name: Check cmdline.txt locations
  stat:
    path: "{{ item }}"
  register: cmdline_check
  loop:
    - /boot/firmware/cmdline.txt
    - /boot/cmdline.txt

- name: Set cmdline file path
  set_fact:
    cmdline_file: "{{ item.stat.path }}"
  when: item.stat.exists
  loop: "{{ cmdline_check.results }}"

- name: Check for cgroup settings
  slurp:
    src: "{{ cmdline_file }}"
  register: cmdline_content
  when: cmdline_file is defined

- name: Add cgroup settings if missing
  lineinfile:
    path: "{{ cmdline_file }}"
    backrefs: yes
    regexp: "^(.*)$"
    line: '\1 cgroup_enable=memory cgroup_memory=1'
  when:
    - cmdline_file is defined
    - "'cgroup_enable=memory' not in (cmdline_content.content | b64decode)"
    - "'cgroup_memory=1' not in (cmdline_content.content | b64decode)"
  register: cgroup_updated

- name: Warn about reboot requirement
  debug:
    msg: "REBOOT REQUIRED: cgroups were configured. Please reboot and re-run the playbook."
  when: cgroup_updated.changed | default(false)
```

---

### Component 5: Main Playbooks

#### [NEW] ansible/playbooks/site.yml

Full provisioning playbook that combines all roles.

```yaml
---
- name: Provision K3s Nodes
  hosts: all
  become: yes

  pre_tasks:
    - name: Display detected running user
      debug:
        msg: "Playbook is being run by: {{ lookup('env', 'USER') }}"

  roles:
    - role: user_setup
    - role: pi_prepare
    - role: k3s_install
      when: k3s_role is defined
```

#### [NEW] ansible/playbooks/user_setup.yml

User-only playbook for isolated user management.

```yaml
---
- name: Setup Users on K3s Nodes
  hosts: all
  become: yes

  roles:
    - role: user_setup
```

---

### Component 6: SSH Public Keys

Both keys are now in place:

- [ddowell.pub](../public_ssh_keys/ddowell.pub)
- [tbigelow.pub](../public_ssh_keys/tbigelow.pub)

---

### Component 7: Docker Testing Infrastructure

#### [NEW] testing/Dockerfile

```dockerfile
FROM ubuntu:22.04

# Install SSH server and sudo
RUN apt-get update && apt-get install -y \
    openssh-server \
    sudo \
    python3 \
    python3-apt \
    && rm -rf /var/lib/apt/lists/*

# Create tbigelow user with passwordless sudo (simulating existing access)
RUN useradd -m -s /bin/bash tbigelow \
    && echo "tbigelow ALL=(ALL) NOPASSWD: ALL" > /etc/sudoers.d/tbigelow \
    && chmod 0440 /etc/sudoers.d/tbigelow

# Setup SSH for tbigelow
RUN mkdir -p /home/tbigelow/.ssh \
    && chmod 700 /home/tbigelow/.ssh

# Copy SSH public key (build arg)
ARG SSH_PUBKEY
RUN echo "${SSH_PUBKEY}" > /home/tbigelow/.ssh/authorized_keys \
    && chmod 600 /home/tbigelow/.ssh/authorized_keys \
    && chown -R tbigelow:tbigelow /home/tbigelow/.ssh

# Configure SSH daemon
RUN mkdir /run/sshd \
    && sed -i 's/#PasswordAuthentication yes/PasswordAuthentication no/' /etc/ssh/sshd_config \
    && sed -i 's/#PubkeyAuthentication yes/PubkeyAuthentication yes/' /etc/ssh/sshd_config

EXPOSE 22

CMD ["/usr/sbin/sshd", "-D"]
```

#### [NEW] testing/docker-compose.yml

```yaml
version: "3.8"

services:
  test-node:
    build:
      context: .
      args:
        SSH_PUBKEY: ${SSH_PUBKEY:-ssh-ed25519 PLACEHOLDER}
    ports:
      - "2222:22"
    hostname: test-k3s-node
    privileged: true # Required if testing cgroup operations
```

#### [NEW] testing/run_tests.sh

```bash
#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# Validate shell script syntax first
echo "=== Validating shell script syntax ==="
for script in "$PROJECT_ROOT"/remote/*.sh "$PROJECT_ROOT"/ansible/*.sh "$SCRIPT_DIR"/*.sh; do
    if [ -f "$script" ]; then
        echo "Checking: $script"
        bash -n "$script" || { echo "FAILED: $script has syntax errors"; exit 1; }
    fi
done
echo "All scripts passed syntax check"

# Load SSH public key for build
export SSH_PUBKEY=$(cat "$PROJECT_ROOT/../public_ssh_keys/tbigelow.pub")

echo "=== Building test container ==="
docker-compose -f "$SCRIPT_DIR/docker-compose.yml" build

echo "=== Starting test container ==="
docker-compose -f "$SCRIPT_DIR/docker-compose.yml" up -d

echo "=== Waiting for SSH to be ready ==="
sleep 3

echo "=== Creating temporary test inventory ==="
cat > "$SCRIPT_DIR/test_inventory.yml" << EOF
all:
  hosts:
    test-node:
      ansible_host: 127.0.0.1
      ansible_port: 2222
      ansible_user: tbigelow
      ansible_ssh_private_key_file: ~/.ssh/id_ed25519
EOF

echo "=== Running Ansible user_setup playbook ==="
cd "$PROJECT_ROOT/ansible"
ansible-playbook -i "$SCRIPT_DIR/test_inventory.yml" playbooks/user_setup.yml -v

echo "=== Verifying user creation ==="
ssh -p 2222 -o StrictHostKeyChecking=no tbigelow@127.0.0.1 "id ddowell && id tbigelow"

echo "=== Cleanup ==="
docker-compose -f "$SCRIPT_DIR/docker-compose.yml" down
rm "$SCRIPT_DIR/test_inventory.yml"

echo "=== All tests passed! ==="
```

---

### Component 8: Environment Variable Handling

Secrets should be passed via environment variables, with fallback to a `.env` file in the repository root.

#### [NEW] ansible/env_loader.sh

Wrapper script to load environment and run Ansible playbooks.

```bash
#!/bin/bash
set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/../../.." && pwd)"
ENV_FILE="$REPO_ROOT/.env"

# Required environment variables
REQUIRED_VARS=("TS_AUTHKEY" "K3S_TOKEN")

# Load from .env if variables not already set
load_env_file() {
    if [ -f "$ENV_FILE" ]; then
        echo "Loading environment from $ENV_FILE"
        set -a
        source "$ENV_FILE"
        set +a
    fi
}

# Check if a variable is set
check_required_vars() {
    local missing=()
    for var in "${REQUIRED_VARS[@]}"; do
        if [ -z "${!var}" ]; then
            missing+=("$var")
        fi
    done

    if [ ${#missing[@]} -gt 0 ]; then
        echo "Error: Required environment variables are not set:"
        for var in "${missing[@]}"; do
            echo "  - $var"
        done
        echo ""
        echo "Please either:"
        echo "  1. Export them before running: export TS_AUTHKEY=... K3S_TOKEN=..."
        echo "  2. Create a .env file at: $ENV_FILE"
        echo ""
        echo "Example .env file:"
        echo "  TS_AUTHKEY=tskey-auth-..."
        echo "  K3S_TOKEN=my-secure-cluster-token"
        exit 1
    fi
}

# Load .env if exists, then validate
load_env_file
check_required_vars

echo "Environment validated. Running Ansible..."
echo "  TS_AUTHKEY: ${TS_AUTHKEY:0:15}... (truncated)"
echo "  K3S_TOKEN:  ${K3S_TOKEN:0:10}... (truncated)"
echo ""

# Forward all arguments to ansible-playbook
cd "$SCRIPT_DIR"
exec ansible-playbook "$@"
```

#### [NEW] .env.example

Template for users to create their own `.env` file.

```bash
# Tailscale Auth Key (required)
# Get from: https://login.tailscale.com/admin/settings/keys
TS_AUTHKEY=tskey-auth-XXXXXXXXXXXXXXXX

# K3s Cluster Token (required for joining nodes)
# Use a strong, shared secret for all nodes
K3S_TOKEN=my-secure-cluster-token

# Optional: Tailscale Service Name (defaults to svc:chalupa-k3s)
# TS_SERVICE_NAME=svc:chalupa-k3s
```

#### Update .gitignore

Ensure `.env` is not committed:

```gitignore
.env
```

---

## Analysis: install.sh Decomposition

Based on review of [install.sh](../scripts/raspberrypi-k3s/remote/install.sh), here's what could be extracted into Ansible vs. kept in the script:

| Task                   | Lines   | Ansible Candidate? | Recommendation                                |
| ---------------------- | ------- | ------------------ | --------------------------------------------- |
| Root check             | 8-12    | No                 | Keep in script (Ansible handles via `become`) |
| Pi cgroup checks       | 34-67   | **Yes**            | Extract to `pi_prepare` role                  |
| iptables check         | 70-78   | **Yes**            | Extract to `pi_prepare` role (info only)      |
| Tailscale install      | 103-108 | **Yes**            | Could use `ansible.builtin.get_url` + shell   |
| Tailscale auth         | 110-118 | Partial            | Requires interactive token, keep combined     |
| Get Tailscale IP       | 120-163 | No                 | Dynamic, keep in script                       |
| K3s installation       | 166-220 | No                 | Complex conditional logic, keep in script     |
| Tailscale service host | 223-260 | No                 | Depends on Tailscale state, keep in script    |

> [!IMPORTANT]
> **Recommendation**: Keep `install.sh` largely intact for K3s/Tailscale installation. Extract only the idempotent Pi preparation tasks (cgroups, iptables) to Ansible for pre-flight checks. This balances automation while preserving the tested installation logic.

---

## Verification Plan

### Automated Tests

```bash
# 1. Validate shell script syntax (bash -n)
echo "=== Validating shell script syntax ==="
find scripts/ -name "*.sh" -exec bash -n {} \;

# 2. Build and run Docker test container
cd scripts/raspberrypi-k3s/testing
./run_tests.sh

# 3. Verify specific assertions
# - ddowell user exists
# - tbigelow user exists
# - Both have SSH keys deployed
# - Both can sudo without password
```

### Manual Verification

After Docker tests pass:

1. Deploy to a real Pi with:

   ```bash
   cd scripts/raspberrypi-k3s/ansible
   ansible-playbook playbooks/user_setup.yml -l <target_host>
   ```

2. Verify SSH access from both users' machines:
   ```bash
   ssh ddowell@<pi_tailscale_ip> sudo whoami  # Should output 'root'
   ssh tbigelow@<pi_tailscale_ip> sudo whoami # Should output 'root'
   ```

---

## Resolved Decisions

| Question               | Decision                                                                 |
| ---------------------- | ------------------------------------------------------------------------ |
| SSH public keys        | ✅ Both `ddowell.pub` and `tbigelow.pub` are ready in `public_ssh_keys/` |
| Initial bootstrap user | One of the managed users (ddowell or tbigelow) - detected automatically  |
| Secrets management     | Environment variables at runtime, with `.env` file fallback              |

---

## Usage

```bash
# Option 1: Export environment variables directly
export TS_AUTHKEY="tskey-auth-..."
export K3S_TOKEN="my-cluster-token"
cd scripts/raspberrypi-k3s/ansible
./env_loader.sh playbooks/site.yml

# Option 2: Use .env file
cp .env.example .env
# Edit .env with your values
cd scripts/raspberrypi-k3s/ansible
./env_loader.sh playbooks/site.yml

# User-only setup (no K3s install)
./env_loader.sh playbooks/user_setup.yml
```
