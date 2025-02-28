Below is an **English README.md** version incorporating the latest changes, including automatic directory creation, rollback logic (backing up the current file state before rolling back), and subcommands (`start`, `stop`, `run`) to manage the server daemon.

한글 매뉴얼을 보시려면 [여기 - 한국어 매뉴얼](README_KR.md)을 클릭해주세요

---

# Docker Compose Web Control

<img width="947" alt="image" src="https://github.com/user-attachments/assets/000bb175-8a35-4202-abc6-647770b12836" />


## Introduction
**Docker Compose Web Control** is a Golang-based web interface that makes it easy to manage your Docker Compose files, including creating/editing files, saving automatic backups, rolling back to previous versions, and restarting Docker Compose services. It features straightforward user authentication, including admin role management.

## Key Features
- **User Registration & Login** (the very first user automatically receives **admin** privileges)
- Web-based management of **directories and files** within the `./docker-compose-list` folder
  - Create/edit files with a built-in editor
  - **Automatic backups** on every save (a `backups/` folder is created within the same directory)
  - **Rollback** old versions and **download** backups
  - Before rolling back, the **current state** of the file is also **backed up** so you can restore it later if needed
  - Up to **20 backups** are retained (oldest backups are pruned automatically)
- **Docker Compose restart** support (`docker-compose down; docker-compose up -d`)
- **Admin Page** to manage user roles (admin / none)

## Project Structure (Example)
```
.
├── main.go               # Main application code with subcommands (start/stop/run)
├── templates/            # HTML templates
│   ├── landing.html
│   ├── console.html
│   ├── admin.html
│   └── register.html
├── docker-compose-list/            # docker-compose.yml base directory
│   ├── my-docker-compose1
│   ├── ...
│   ├── ..
└── static/               # Static files (CSS, JS, etc.)
```
*(You can split files as needed.)*

## Installation & Build

1. **Clone the repository** (example):
   ```bash
   git clone https://github.com/ralfyang/docker-compose-webctl.git
   cd docker-compose-webctl
   ```

2. **(Optional) Create a `.env` file** for configuration:
   ```bash
   # .env
   port="15500"
   docker_id="YOUR_DOCKER_ID"
   docker_password="YOUR_DOCKER_PASSWORD"
   ```
   - If `port` is not specified, it defaults to `:15500`.

3. **Install dependencies & build**:
   ```bash
   go mod tidy
   go build -o dc_webconsole main.go
   ```
   - This produces the `dc_webconsole` binary in the current directory.

## Running the Server
The **main.go** file supports **subcommands** (`start`, `stop`, `run`) to manage the server:

1. **Foreground (runtime) execution**:
   ```bash
   ./dc_webconsole run
   ```
   - Runs the server in the foreground, logs printed directly to the terminal.

2. **Daemon (background) execution**:
   ```bash
   ./dc_webconsole start
   ```
   - Writes the PID to `dc_webconsole.pid`  
   - Logs are appended to `dc_webconsole.log`

3. **Stop daemon**:
   ```bash
   ./dc_webconsole stop
   ```
   - Reads the PID from `dc_webconsole.pid`, kills the process, and removes the PID file.

> **Note**: The code automatically creates the `docker-compose-list` directory if it does not exist.  
> You can still create it manually if you prefer (e.g., to understand where your files go), but it’s optional.

## Usage
1. **Open** a browser and navigate to [http://localhost:15500](http://localhost:15500) (or the port set in `.env`)
2. **Register** an account (the first user is automatically **admin**)
3. After logging in, access `/console`:
   - **Create or select** a directory  
   - **Create or select** a Compose file  
   - Edit and click **Save** → automatic backup  
   - Or **Save & Restart** → also restarts Docker Compose  
   - **Check backup list** for historical versions; download or roll back
   - During rollback, the current file state is also **saved as a new backup** before reverting
4. **Admin page** (`/console/admin`) is available only to admin users:
   - Update user roles (admin or none)

## Rollback Logic
By default, when rolling back to a previous backup, **the current state** of the file is **backed up first** to preserve it. This means you can always revert the rollback if needed. If you look at the `rollbackFileAPI`, you’ll see a call to `backupFile(...)` right before overwriting with the chosen backup file.

## Notes
- Ensure **Docker** and **docker-compose** are installed on your system.
- The maximum **20 backups** are stored in `<directory>/backups/`; older files are automatically pruned.
- If Docker Compose needs privileges, you may require **root** or Docker group membership to run it.

## Contributing
1. **Fork** the repository
2. Create a new branch (`git checkout -b feature/new-feature`)
3. Commit your changes (`git commit -m 'Add new feature'`)
4. Push the branch (`git push origin feature/new-feature`)
5. Create a Pull Request

## License
This project is licensed under the **MIT License**. See [LICENSE](LICENSE) for details.
