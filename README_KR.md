# Docker Compose Web Control

## 소개
Docker Compose Web Control은 웹 인터페이스를 통해 Docker Compose 파일을 관리하고 시스템을 제어할 수 있는 도구입니다. 이 애플리케이션은 Golang과 Gin 프레임워크를 사용하여 제작되었으며, 간단한 사용자 인증 및 권한 관리를 포함하고 있습니다.

## 주요 기능
- 회원가입 및 로그인 기능 제공
- 첫 번째 사용자가 자동으로 어드민 권한 부여
- 어드민 권한을 통해 사용자 권한을 관리
- 특정 디렉토리 내의 Docker Compose 파일 관리
- Docker Compose 파일 편집기 제공
- 파일 저장 시 자동 백업 및 버전 관리 (최대 20개 유지)
- 백업 파일 다운로드 및 롤백 기능 제공
- Docker Compose 재시작 기능 (docker-compose down/up)

## 프로젝트 구조
```
.
├── main.go                # 메인 애플리케이션 진입점
├── account.go             # 사용자 계정 관리
├── auth.go                # 인증 및 인가 미들웨어
├── console.go             # Docker Compose 웹 콘솔
├── admin.go               # 어드민 페이지 및 권한 관리
├── backup.go              # 파일 백업 및 Docker 재시작 로직
├── templates/             # HTML 템플릿 폴더
│   ├── landing.html       # 랜딩 페이지
│   ├── console.html       # Docker Compose 웹 콘솔
│   ├── admin.html         # 어드민 페이지
│   └── register.html      # 회원가입 페이지
└── static/                # CSS, JS 및 프론트엔드 파일
    ├── style.css          # 스타일 시트
    └── app.js             # 클라이언트 로직
```

## 설치 및 실행 방법
### 1. 프로젝트 클론
```sh
git clone https://github.com/yourusername/docker-compose-webctl.git
cd docker-compose-webctl
```

### 2. 필요한 디렉토리 생성
```sh
mkdir docker-compose-list
mkdir backups
```

### 3. 의존성 설치
```sh
go mod tidy
```

### 4. 서버 실행
```sh
go run main.go
```

서버는 기본적으로 `http://localhost:15500` 에서 실행됩니다.

## 사용 방법
1. 웹 브라우저에서 `http://localhost:15500` 접속
2. 회원가입 후 로그인
3. Docker Compose 웹 콘솔을 통해 파일을 생성, 편집, 백업 및 롤백 가능
4. 어드민 계정으로 사용자 권한 관리 가능

## 주의사항
- Docker Compose 파일은 `./docker-compose-list` 하위 디렉토리에서만 접근 가능합니다.
- Sudo 권한이 필요한 명령어를 사용해야 하므로, 서버는 적절한 권한으로 실행되어야 합니다.
- 자동으로 생성되는 백업 파일은 최대 20개까지만 유지되며, 오래된 파일은 자동으로 삭제됩니다.

## 기여 방법
1. Fork 저장소
2. 새로운 브랜치 생성 (`git checkout -b feature/새기능`)
3. 변경 사항 커밋 (`git commit -m '새 기능 추가'`)
4. 브랜치 푸시 (`git push origin feature/새기능`)
5. Pull Request 생성

## 라이선스
이 프로젝트는 MIT 라이선스를 따릅니다.

