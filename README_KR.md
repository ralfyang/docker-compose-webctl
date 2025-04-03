아래는 **`docker-compose-list` 디렉토리가 자동 생성되는 점**을 반영하여 작성한 **최신 README.md** 예시입니다.  
프로젝트 환경이나 디렉토리 구조에 맞춰 적절히 수정해서 사용하세요.

---

# Docker Compose Web Control

<img width="947" alt="image" src="https://github.com/user-attachments/assets/000bb175-8a35-4202-abc6-647770b12836" />


## 소개
**Docker Compose Web Control**은 웹 브라우저로 Docker Compose 파일을 손쉽게 관리하고, 작업 내용을 백업/롤백하며, Docker Compose를 재시작할 수 있도록 해주는 도구입니다.  
Golang과 Gin 프레임워크를 기반으로 작성되었으며, 간단한 인증/인가 로직과 **파일 백업** 기능을 제공합니다.

## 주요 기능
- **회원가입 & 로그인** (첫 번째 가입자는 자동으로 **어드민(Admin)** 권한 획득)
- **디렉토리/파일** 웹 관리 (기본 디렉토리: `./docker-compose-list`)
  - 디렉토리/파일 생성, 내용 편집
  - 파일 **저장 시 자동 백업** (동일 디렉토리에 `backups/` 폴더)
  - **롤백 전** 최신 상태도 추가로 백업하여 언제든 복원 가능
  - **최대 20개** 백업만 유지 (자동 순환)
- **Docker Compose 재시작** (docker-compose down; up -d)
- **어드민** 페이지에서 사용자 권한 관리 (admin/none)

## 디렉토리 구조 예시
```
.
├── main.go               # 전체 로직 (start/stop/run 서브커맨드 포함)
├── templates/            # HTML 템플릿
│   ├── landing.html
│   ├── console.html
│   ├── admin.html
│   └── register.html
├── docker-compose-list/            # 관리 하고자 하는 docker-compose.yml 파일들의 있는 베이스 디렉토리 
│   ├── my-docker-compose1
│   ├── ...
│   ├── ..
└── static/               # 정적 파일 (CSS, JS 등)
```

> 필요에 따라 파일 분할 가능.

## 설치 & 빌드
*설치에 있어서 golang의 설치가 필요합니다: Golang - https://go.dev/dl*

1. **소스 클론** (예시)
   ```bash
   git clone https://github.com/ralfyang/docker-compose-webctl.git
   cd docker-compose-webctl
   ```

2. **`.env` 파일 생성(옵션)**
   ```bash
   # .env
   port="15500"
   docker_id="YOUR_DOCKER_ID"
   docker_password="YOUR_DOCKER_PASSWORD"
   ```
   - 설정하지 않으면 `port`는 기본 `:15500` 사용.

3. **의존성 정리 & 빌드**
   ```bash
   go mod tidy
   go build -o dc_webconsole main.go
   ```
   - 빌드 후 `dc_webconsole` 실행 파일 생성

## 실행 방법 (서브커맨드)
아래 **서브커맨드**로 서버를 실행/중지할 수 있습니다.

1. **Foreground (런타임)**
   ```bash
   ./dc_webconsole run
   ```
   - 터미널에 로그를 표시하며 서버가 동작합니다.

2. **데몬(백그라운드) 실행**
   ```bash
   ./dc_webconsole start
   ```
   - `dc_webconsole.pid` 파일에 PID가 기록되고, `dc_webconsole.log`에 로그가 저장됩니다.

3. **데몬 중지**
   ```bash
   ./dc_webconsole stop
   ```
   - `dc_webconsole.pid`에 있는 PID를 찾아 프로세스를 종료합니다.

> **참고**: `main.go` 코드에서 `baseDir`(`docker-compose-list`)가 **없으면 자동 생성**합니다.  
> 직접 `mkdir docker-compose-list`를 할 필요는 없지만, 구조를 이해하기 위해 수동 생성해도 무방합니다.

## 사용 방법
1. **브라우저**에서 [http://localhost:15500](http://localhost:15500) (또는 `.env`에 설정한 포트) 접속
2. **회원가입** (첫 사용자: 자동 **admin** 권한)
3. 로그인 후, **/console** 화면에서:
   - **디렉토리 생성** 혹은 기존 디렉토리 클릭
   - **파일 생성** 혹은 기존 파일 클릭
   - 내용 편집 후 “저장” → **자동 백업**, “저장 & 리스타트” → **도커 재시작**
   - **백업 목록**에서 기존 버전 확인, 다운로드, 롤백 가능  
   - 롤백 시 “현재 파일 상태”도 먼저 백업하여, 추후 원복 가능
4. **관리자(Admin)** 계정으로 `/console/admin` 접근:
   - 다른 사용자들의 권한을 “admin” 또는 “none”으로 변경 가능

## 롤백 시 주의사항
- **롤백**(`rollbackFileAPI`) 로직은 “과거 백업본”으로 복원하기 전, **현재 상태**를 **새 백업**으로 저장합니다.  
  즉, 롤백 전 상태도 별도의 백업 파일로 남아, 언제든 다시 복원할 수 있습니다.

## 주의 사항
- **Docker** 및 **docker-compose**가 사전에 설치되어 있어야 합니다.
- **파일이 위치한 디렉토리 내** `backups/` 폴더가 자동 생성되며, 최대 20개 백업만 유지됩니다.
- Docker Compose 재시작 시 권한 문제가 있을 수 있으므로, **root 또는 Docker 권한** 확보 필요.

## 기여 방법
1. 저장소를 **Fork**합니다.
2. 새 브랜치 생성(`git checkout -b feature/new-feature`).
3. 변경사항 커밋(`git commit -m 'Add new feature'`).
4. 브랜치를 푸시(`git push origin feature/new-feature`).
5. Pull Request 생성.

## 라이선스
이 프로젝트는 **MIT License**로 배포됩니다.  
자세한 내용은 [LICENSE](LICENSE) 파일을 참고하세요.
