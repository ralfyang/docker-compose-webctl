# Docker-Compose Web Console

This project provides a **Go (Gin framework)**-based web console to manage **docker-compose** with ease.  
It allows users to sign up, log in, create/edit configuration files, restart docker-compose services, and back up or roll back changes.

## Key Features

1. **Sign Up / Log In**  
   - The very first registered user automatically gains **admin** privileges.  
   - Subsequent users have `none` privileges until the admin assigns them as `admin`.

2. **Docker-Compose Web Console**  
   - **Directory Management**: Create directories under `docker-compose-list`.  
   - **File Management**: Within each directory, create configuration files (e.g., `.yml`).  
   - **File Editing**: A web-based editor to modify and save files.  
   - **Save & Restart**: Execute `docker-compose -f [file] down; sleep 2; up -d` to restart containers.

3. **Backup / Rollback**  
   - Each file edit triggers a backup (keeping up to 20 versions).  
   - Users can view a list of backups, then roll back to a chosen version and restart.  
   - Backup files can be **downloaded** as well.

4. **Admin Page**  
   - A page to manage user roles (`admin` or `none`).  
   - The admin page is accessible via a button in the top-right corner of the web console (for admin users only).

5. **UI/UX**  
   - **Landing Page**: A login form with a “Sign Up” button for new users.  
   - **Automatic Login** after sign-up. The first user is `admin`; others need admin approval for elevated privileges.  
   - **Web Console**:  
     - A directory list → select a directory  
     - A file list → select a file  
     - An editor for viewing/editing the file content  
     - “Save” and “Save & Restart” buttons  
     - Backup list & rollback function  
     - An **Admin** button (if the user is an admin) at the top-right corner  
     - A **Logout** button at the bottom-right corner.

## Directory Structure
your-project/ 
├─ main.go
└─ templates/
├─ landing.html # Landing page (login form + sign-up link)
├─ register.html # Sign-up form
├─ console.html # Main Docker-Compose Web Console UI
└─ admin.html # Admin page (user role management)


- **`main.go`**:  
  - The Go + Gin server  
  - Routing (login, sign-up, `/console`, `/console/api/*`, etc.)  
  - Invokes `docker-compose` via `exec.Command("docker-compose", ...)`  
  - Manages file backups, rollback, and user authentication

- **`templates/`** folder:  
  - Contains all HTML templates  
  - Loaded with `r.LoadHTMLGlob("templates/*.html")` in `main.go`  
  - Rendered via `c.HTML(...)`

1. **Clone** the repository:
   ```bash
   git clone https://github.com/username/docker-compose-webctl.git
   cd docker-compose-webctl
