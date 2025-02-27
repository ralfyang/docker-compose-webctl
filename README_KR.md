아래는 **한국어**로 작성된 `README.md` 예시입니다. `.env` 파일을 사용한 설정, **단일 실행 파일**(`dc_webconsole`)로 빌드 및 서브커맨드(`start`, `stop`, `run`)를 통한 서버 구동 방법, 그리고 **파일이 위치한 디렉토리에 자동 생성되는 `backups` 폴더** 등의 내용을 포함하고 있습니다. 원하시는 대로 경로나 레포지토리 정보를 수정하셔서 사용하세요.

---

# Docker Compose Web Control

![image](https://github.com/user-attachments/assets/e145d51e-2134-4e3d-bc2b-664be171ea14)

## 소개
**Docker Compose Web Control**은 웹 인터페이스를 통해 Docker Compose 파일을 관리하고 시스템을 제어할 수 있도록 해주는 도구입니다. Golang과 Gin 프레임워크로 작성되었으며, 간단한 사용자 인증/인가(권한) 기능을 제공합니다.

## 주요 기능
- **회원가입 & 로그인** 기능  
- 첫 번째 가입자는 **자동으로 어드민**(admin) 권한  
- 어드민 페이지에서 **유저 권한**을 관리 가능  
- 특정 디렉토리(`./docker-compose-list`) 안에 있는 **Docker Compose 파일**을 웹에서 관리  
- **웹 기반 에디터**로 Compose 파일 편집  
- **자동 백업** 및 버전 관리 (파일 저장 시 이전 버전을 **backups** 폴더에 저장, 최대 20개 유지)  
- **백업 다운로드/롤백** 기능  
- **docker-compose restart**(down → up -d) 기능

## 디렉토리 구조
```
.
├── main.go                # 메인 실행 파일 진입점
├── account.go             # (예시) 계정 관련 로직
├── auth.go                # 인증/인가 미들웨어
├── console.go             # Docker Compose 웹 콘솔 API
├── admin.go               # 어드민(관리자) 페이지
├── backup.go              # 파일 백업 및 Docker 재시작 로직
├── templates/             # HTML 템플릿 폴더
│   ├── landing.html
│   ├── console.html
│   ├── admin.html
│   └── register.html
└── static/                # CSS, JS 등 정적 리소스
    ├── style.css
    └── app.js
```

> 실제 소스 구조는 필요에 따라 다를 수 있습니다.

## 설치 & 사용 방법

### 1. 저장소 클론
```sh
git clone https://github.com/yourusername/docker-compose-webctl.git
cd docker-compose-webctl
```

### 2. `.env` 파일 생성/설정
프로젝트 루트(예: `main.go`가 있는 경로)에 `.env` 파일을 생성하고, 아래처럼 작성하세요:
```bash
port="15500"
docker_id="YOUR_DOCKER_ID"
docker_password="YOUR_DOCKER_PASSWORD"
```
- **`port`**: 서버가 사용할 포트 번호 (미설정 시 기본값 15500)  
- **`docker_id`, `docker_password`**: 필요하다면 Docker 레지스트리에 로그인할 때 사용 (코드에서 구현 여부에 따라 사용)

### 3. 필요한 디렉토리 생성
```sh
mkdir docker-compose-list
```
- 이 디렉토리 안에 Docker Compose 파일/디렉토리를 저장하고 관리합니다.
- **주의**: 예전에는 전역 `./backups` 폴더를 사용했지만, 이제는 **각 디렉토리 내**에 `backups` 폴더가 자동 생성됩니다.  
  예: `docker-compose-list/testtt/aaa.yml` 편집 시, `docker-compose-list/testtt/backups/` 폴더가 생성됨.

### 4. 빌드
```sh
go mod tidy                     # 의존성 정리
go build -o dc_webconsole main.go
```
- 빌드 후 `dc_webconsole` 실행 파일이 생성됩니다.

### 5. 서버 실행
**3가지 서브커맨드**를 통해 실행/중지/런타임 모드를 선택할 수 있습니다:

1. **포그라운드(런타임) 실행**  
   ```bash
   ./dc_webconsole run
   ```
   - 서버가 터미널에서 바로 로그를 보여주며 실행됩니다.

2. **데몬(백그라운드) 실행**  
   ```bash
   ./dc_webconsole start
   ```
   - 백그라운드에서 서버가 실행됩니다.
   - `dc_webconsole.pid` 파일에 PID가 기록되고, `dc_webconsole.log` 파일에 로그가 쌓입니다.

3. **데몬 중지**  
   ```bash
   ./dc_webconsole stop
   ```
   - `dc_webconsole.pid`에 적힌 PID로 프로세스를 찾아 종료합니다.
   - 종료 후 `dc_webconsole.pid` 파일을 삭제합니다.

> 기본 포트는 `:15500`이며, `.env`에서 `port="12345"` 형태로 변경할 수 있습니다.

### 6. 웹 인터페이스 사용
브라우저에서 [http://localhost:15500](http://localhost:15500) (또는 설정한 포트)에 접속하세요.

1. **회원가입** 후 **로그인**  
   - **첫 사용자**는 자동으로 어드민 권한이 부여됩니다.
2. **웹 콘솔**에서:
   - 원하는 디렉토리를 만들거나 선택 (예: `testtt`)
   - Docker Compose 파일(`testtt/aaa.yml`) 등을 생성/선택
   - 내용 수정 후 **저장** 버튼  
     - 저장 시 기존 파일이 `testtt/backups/aaa_YYYYmmdd_HHMMSS.yml` 형태로 백업
3. **백업**  
   - **“백업 목록”** 버튼을 누르면 해당 파일(`aaa.yml`)의 백업을 확인  
   - 백업 다운로드/롤백이 가능합니다 (최대 20개 보관, 오래된 것은 자동 삭제)
4. **어드민 페이지**  
   - **admin** 권한을 가진 계정으로 로그인 시 `/console/admin` 에 접속 가능  
   - 다른 사용자들의 **Role**(권한)을 “admin” 또는 “none”으로 변경

## 주의 사항
- **데몬 모드**로 실행 중 로그는 `dc_webconsole.log` 파일에 저장됩니다. 실시간으로 확인하려면:
  ```sh
  tail -f dc_webconsole.log
  ```
- **각 파일**이 위치한 디렉토리마다 **로컬 `backups/` 폴더**가 자동 생성되며, 20개 이상의 백업이 저장되면 오래된 순으로 삭제됩니다.
- **Docker Compose**를 사용하려면 시스템에 Docker 및 docker-compose가 **사전에 설치**되어 있어야 합니다.
- `docker-compose down; docker-compose up -d` 명령으로 재시작하므로, **root 권한**(또는 Docker 권한)이 필요할 수 있습니다.

## 기여 방법
1. 저장소를 **포크**(Fork)합니다.  
2. 새로운 브랜치를 생성합니다. (`git checkout -b feature/new-feature`)  
3. 변경사항을 커밋합니다. (`git commit -m 'Add new feature'`)  
4. 브랜치를 푸시합니다. (`git push origin feature/new-feature`)  
5. Pull Request를 생성합니다.

## 라이선스
이 프로젝트는 **MIT License**로 배포됩니다. 자세한 사항은 [LICENSE](LICENSE) 파일을 참고하세요.
