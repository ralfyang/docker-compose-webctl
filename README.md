# Docker Compose Web Control

## Introduction
Docker Compose Web Control is a tool that allows you to manage Docker Compose files and control the system through a web interface. This application is built using Golang and the Gin framework, featuring simple user authentication and authorization management.

## Key Features
- User registration and login functionality
- The first registered user automatically receives admin privileges
- Admin can manage user roles via a dedicated interface
- Manage Docker Compose files within a specific directory
- Provides a web-based editor for Docker Compose files
- Automatic backup and version control when saving files (up to 20 versions maintained)
- Download and rollback functionality for backup files
- Docker Compose restart functionality (docker-compose down/up)

## Project Structure
```
.
├── main.go                # Main application entry point
├── account.go             # User account management
├── auth.go                # Authentication and authorization middleware
├── console.go             # Docker Compose web console
├── admin.go               # Admin page and role management
├── backup.go              # File backup and Docker restart logic
├── templates/             # HTML templates folder
│   ├── landing.html       # Landing page
│   ├── console.html       # Docker Compose web console
│   ├── admin.html         # Admin page
│   └── register.html      # Registration page
└── static/                # CSS, JS, and frontend files
    ├── style.css          # Stylesheet
    └── app.js             # Client-side logic
```

## Installation & Execution
### 1. Clone the Project
```sh
git clone https://github.com/yourusername/docker-compose-webctl.git
cd docker-compose-webctl
```

### 2. Create Required Directories
```sh
mkdir docker-compose-list
mkdir backups
```

### 3. Install Dependencies
```sh
go mod tidy
```

### 4. Run the Server
```sh
go run main.go
```

The server will run at `http://localhost:15500` by default.

## How to Use
1. Open `http://localhost:15500` in your web browser
2. Register and log in
3. Create, edit, backup, and rollback Docker Compose files via the web console
4. Manage user roles through the admin interface (admin-only)

## Caution
- Only access files within the `./docker-compose-list` directory.
- Server must run with sufficient permissions as sudo is required for certain operations.
- A maximum of 20 backup files are maintained; older files are automatically deleted.

## Contribution
1. Fork the repository
2. Create a new branch (`git checkout -b feature/new-feature`)
3. Commit changes (`git commit -m 'Add new feature'`)
4. Push the branch (`git push origin feature/new-feature`)
5. Create a Pull Request

## License
This project is licensed under the MIT License.

