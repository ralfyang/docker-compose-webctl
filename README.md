Below is an updated **README.md** that includes instructions for building the binary (`dc_webconsole`), using the **`.env`** file for configuration, and running the server with the **subcommands** (`start`, `stop`, `run`). It also describes the new **per-directory `backups` folder** behavior. Adjust paths and repository links as needed.

---

# Docker Compose Web Control

![image](https://github.com/user-attachments/assets/e145d51e-2134-4e3d-bc2b-664be171ea14)

## Introduction
**Docker Compose Web Control** is a tool that allows you to manage Docker Compose files and control the system through a web interface. This application is built using **Golang** and the **Gin** framework, featuring simple user authentication and authorization management.

## Key Features
- **User registration and login** functionality  
- The first registered user automatically receives **admin** privileges  
- Admin can manage user roles via a dedicated admin interface  
- Manage Docker Compose files within a **`./docker-compose-list`** directory (by default)  
- **Web-based editor** for Docker Compose files  
- **Automatic backup** and version control when saving files:  
  - **Backups** are stored in a `backups/` folder **within the same directory** as the file being edited  
  - A maximum of **20** versions are maintained (oldest are auto-deleted)  
- **Rollback** and **download** functionality for backup files  
- **Docker Compose restart** functionality (`docker-compose down/up`)

## Project Structure
```
.
├── main.go                # Main application entry point
├── account.go             # User account management (example; you may have merged code)
├── auth.go                # Authentication and authorization middleware
├── console.go             # Docker Compose web console endpoints
├── admin.go               # Admin page and role management
├── backup.go              # File backup and Docker restart logic
├── templates/             # HTML templates folder
│   ├── landing.html
│   ├── console.html
│   ├── admin.html
│   └── register.html
└── static/                # CSS, JS, and other static frontend files
    ├── style.css
    └── app.js
```

*(Your actual structure may vary depending on how you’ve split files.)*

## Installation & Usage

### 1. Clone the Project
```sh
git clone https://github.com/yourusername/docker-compose-webctl.git
cd docker-compose-webctl
```

### 2. Create or Update Your `.env` File
Create a **`.env`** file in the project root (same directory as `main.go`) with content like:
```bash
port="15500"
docker_id="YOUR_DOCKER_ID"
docker_password="YOUR_DOCKER_PASSWORD"
```
- **`port`**: The port where the server will listen. (Default: `15500` if unset)  
- **`docker_id`**, `docker_password`: Optional fields you can use if you have Docker registry login logic (not strictly required unless implemented in your code).

### 3. Create Required Folders
By default:
```sh
mkdir docker-compose-list
```
- This is where your Docker Compose directories/files will be managed.
- **Note**: You do *not* need a global `./backups` folder anymore. When you edit/save a file like `testtt/aaa.yml`, it will create a `testtt/backups` folder **in the same directory** automatically.

### 4. Build the Binary
Compile the Go code to produce an executable named `dc_webconsole` (or whatever name you prefer):
```sh
go mod tidy              # ensure dependencies are present
go build -o dc_webconsole main.go
```

### 5. Run the Server
There are **three modes** available (subcommands):

1. **Foreground (run)**  
   ```
   ./dc_webconsole run
   ```
   - Runs the server in the foreground. Logs will appear in the console.

2. **Start (daemon)**  
   ```
   ./dc_webconsole start
   ```
   - Runs the server **in the background** (daemon mode).
   - Creates a `dc_webconsole.pid` file containing the daemon’s PID and appends logs to `dc_webconsole.log`.

3. **Stop (daemon)**  
   ```
   ./dc_webconsole stop
   ```
   - Reads the PID from `dc_webconsole.pid`, kills the corresponding process, and removes `dc_webconsole.pid`.

> **Default port** is `:15500` unless overridden by `port="...."` in `.env`.

### 6. Access the Web Interface
Open your browser to [http://localhost:15500](http://localhost:15500) (or whichever port you configured).

## How to Use
1. **Open** `http://localhost:15500` in your web browser.
2. **Register** an account and log in.
   - The **first** registered user automatically becomes an **admin**.
3. Use the **web console** to:
   - Create and select a directory (e.g., `testtt`).
   - Create or select a Docker Compose file (e.g., `testtt/aaa.yml`).
   - Edit the file contents and click **Save**.
   - Each save will automatically back up the old version to `testtt/backups/`.
4. **Backups**:  
   - Clicking **“백업 목록”(Backup List)** shows the available backups for the *currently open file*.  
   - You can **download** or **roll back** any of these backups.  
   - Up to 20 older backups are maintained.  
5. **Admin page**:  
   - If you’re **admin**, you can visit the admin page (`/console/admin`) to manage user roles.

## Notes
- The **daemon mode** logs everything to `dc_webconsole.log`. You can tail this file to see server logs in real time:
  ```sh
  tail -f dc_webconsole.log
  ```
- Each **Docker Compose file** has its **own** `backups/` folder in the **same directory**. For example:
  ```
  docker-compose-list/
  ├── testtt/
  │   ├── aaa.yml
  │   └── backups/
  │       ├── aaa_20250301_120000.yml
  │       ├── aaa_20250301_120100.yml
  │       └── ...
  ```
- **Docker Compose restart** uses `docker-compose -f <filename> down` followed by `up -d`. Ensure your host system has Docker and Docker Compose installed.

## Contribution
1. Fork the repository.
2. Create a new branch (`git checkout -b feature/new-feature`).
3. Commit changes (`git commit -m 'Add new feature'`).
4. Push the branch (`git push origin feature/new-feature`).
5. Create a Pull Request.

## License
This project is licensed under the **MIT License**. See [LICENSE](LICENSE) for details.
