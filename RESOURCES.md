# Praetor Supported Resources Reference

Praetor comes out-of-the-box with native resource types designed to manage the configuration and state of Linux hosts. Each resource is defined using a declarative schema and validated on both the master and agent sides.

---

## 1. File
Manages files, their existence, content, permissions, and ownership on the filesystem.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `path` | `string` | Absolute path to the file on the host. | Yes | Must be a valid path |
| `ensure` | `string` | Desired presence state. | Yes | `present`, `absent` |
| `content` | `string` | The text content of the file. Supports Go template hydration. | No | |
| `mode` | `string` | Posix file permission mode (octal representation). | No | e.g. `"0644"`, `"0755"` |
| `owner` | `string` | Owner username or UID. | No | |
| `group` | `string` | Group name or GID. | No | |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: File
metadata:
  name: dynamic-motd
spec:
  path: /etc/motd
  ensure: present
  content: "Welcome to host {{ .facts.hostname }} running {{ .facts.os }}!"
  mode: "0644"
  owner: root
  group: root
```

---

## 2. Package
Manages OS packages using the native package manager (`apt`, `yum`, `apk`, etc.) automatically detected by the Agent.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `name` | `string` | The name of the package. | Yes | |
| `ensure` | `string` | Desired presence state. | Yes | `present`, `absent`, `latest` |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: Package
metadata:
  name: install-nginx
spec:
  name: nginx
  ensure: present
```

---

## 3. Service
Manages system services (daemons), typically running under `systemd` or standard `service` tools.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `name` | `string` | The name of the service/daemon. | Yes | |
| `ensure` | `string` | State of execution. | Yes | `running`, `stopped` |
| `enable` | `bool` | Whether the service should boot/start automatically on system startup. | No | `false` |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: Service
metadata:
  name: start-nginx-service
spec:
  name: nginx
  ensure: running
  enable: true
```

---

## 4. Exec
Executes arbitrary commands on the agent node. Includes idempotency controls to prevent executing on every run.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `command` | `string` | The shell command to run. | Yes | |
| `onlyif` | `string` | Run command only if this check command returns exit status `0`. | No | |
| `unless` | `string` | Run command unless this check command returns exit status `0`. | No | |
| `creates` | `string` | Run command only if this file path does not exist. | No | |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: Exec
metadata:
  name: generate-dhparam
spec:
  command: "openssl dhparam -out /etc/nginx/dhparam.pem 2048"
  creates: /etc/nginx/dhparam.pem
```

---

## 5. User
Manages OS user accounts natively via `useradd`, `usermod`, and `userdel`.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `ensure` | `string` | Desired presence state. | No | `present` (default if omitted), `absent` |
| `uid` | `int` | Explicit UID to assign to the user. | No | |
| `gid` | `int` | Primary GID or group name. | No | |
| `groups` | `[]string` | List of secondary group names. | No | |
| `shell` | `string` | Path to the default login shell. | No | |
| `home` | `string` | Path to the home directory. | No | |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: User
metadata:
  name: deploy-user
spec:
  ensure: present
  uid: 1005
  groups:
    - sudo
    - docker
  shell: /bin/bash
  home: /home/deployer
```

---

## 6. Group
Manages OS user groups natively via `groupadd`, `groupmod`, and `groupdel`.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `ensure` | `string` | Desired presence state. | No | `present` (default if omitted), `absent` |
| `gid` | `int` | Explicit GID to assign to the group. | No | |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: Group
metadata:
  name: custom-admin-group
spec:
  ensure: present
  gid: 9000
```

---

## 7. Cron
Manages cron jobs within a user's crontab using isolated tracking IDs to ensure safe edits.

### Schema Spec
| Field | Type | Description | Required | Default / Validation |
| :--- | :--- | :--- | :--- | :--- |
| `user` | `string` | Owner of the target crontab. | No | `root` (default) |
| `command` | `string` | The task/command to execute. | Yes | |
| `ensure` | `string` | Desired presence state. | No | `present` (default), `absent` |
| `minute` | `string` | Minute expression. | No | `*` (default) |
| `hour` | `string` | Hour expression. | No | `*` (default) |
| `day_of_month`| `string` | Day of month expression. | No | `*` (default) |
| `month` | `string` | Month expression. | No | `*` (default) |
| `day_of_week` | `string` | Day of week expression. | No | `*` (default) |

### Example YAML
```yaml
apiVersion: praetor.io/v1alpha1
kind: Cron
metadata:
  name: backup-job
spec:
  user: root
  command: "/usr/local/bin/backup.sh > /dev/null 2>&1"
  minute: "0"
  hour: "2"
  day_of_week: "*"
```
