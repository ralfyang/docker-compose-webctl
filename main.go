package main

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "io/ioutil"
    "log"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "sort"
    "strconv"
    "strings"
    "time"

    "github.com/gin-contrib/sessions"
    "github.com/gin-contrib/sessions/cookie"
    "github.com/gin-gonic/gin"
    "github.com/joho/godotenv"
    "golang.org/x/crypto/bcrypt"
)

// ======================================================
// 1. 전역 설정
// ======================================================

type User struct {
    Email    string
    Password string // bcrypt 해시
    Role     string // "admin" 또는 "none"
}

var users = make(map[string]*User)

const accountFile = ".account"

// docker-compose 파일이 저장될 디렉토리
var baseDir = "./docker-compose-list"

// 데몬 동작 시 사용하는 PID/로그 파일
const pidFile = "dc_webconsole.pid"
const logFile = "dc_webconsole.log"

// ======================================================
// 2. 계정(.account) 로드/저장
// ======================================================

var firstRegisteredUserEmail string

func loadAccounts() error {
    f, err := os.Open(accountFile)
    if err != nil {
        if os.IsNotExist(err) {
            return nil // .account 파일이 없으면 그냥 반환
        }
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    lineCount := 0
    for scanner.Scan() {
        line := scanner.Text()
        fields := strings.Split(line, ",")
        if len(fields) < 3 {
            continue
        }
        email := fields[0]
        password := fields[1]
        role := fields[2]

        // 첫 번째 라인이라면 firstRegisteredUserEmail 설정
        lineCount++
        if lineCount == 1 && firstRegisteredUserEmail == "" {
            firstRegisteredUserEmail = email
        }

        users[email] = &User{Email: email, Password: password, Role: role}
    }
    return scanner.Err()
}


func saveAccounts() error {
    f, err := os.Create(accountFile)
    if err != nil {
        return err
    }
    defer f.Close()

    for _, u := range users {
        line := fmt.Sprintf("%s,%s,%s\n", u.Email, u.Password, u.Role)
        if _, err := f.WriteString(line); err != nil {
            return err
        }
    }
    return nil
}

func createUser(email, hashedPwd, role string) error {
    if _, exist := users[email]; exist {
        return errors.New("이미 등록된 이메일입니다.")
    }
    users[email] = &User{Email: email, Password: hashedPwd, Role: role}
    return saveAccounts()
}

// ======================================================
// 3. Landing Page & 로그인/회원가입
// ======================================================

func landingPage(c *gin.Context) {
    c.HTML(http.StatusOK, "landing.html", nil)
}

func doLogin(c *gin.Context) {
    email := c.PostForm("email")
    pw := c.PostForm("password")

    user, ok := users[email]
    if !ok {
        c.String(http.StatusUnauthorized, "등록되지 않은 이메일입니다.")
        return
    }
    // 비밀번호 검증
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pw)); err != nil {
        c.String(http.StatusUnauthorized, "비밀번호가 일치하지 않습니다.")
        return
    }

    sess := sessions.Default(c)
    sess.Set("user_email", email)
    sess.Save()

    // 로그인 후 콘솔 페이지로 이동
    c.Redirect(http.StatusFound, "/console")
}

func showRegister(c *gin.Context) {
    c.HTML(http.StatusOK, "register.html", nil)
}

func doRegister(c *gin.Context) {
    email := c.PostForm("email")
    pw := c.PostForm("password")
    if email == "" || pw == "" {
        c.String(http.StatusBadRequest, "이메일과 비밀번호가 필요합니다.")
        return
    }
    hashed, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
    if err != nil {
        c.String(http.StatusInternalServerError, "비밀번호 해싱 오류")
        return
    }

    // 첫 사용자 -> admin
    role := "none"
    if len(users) == 0 {
        role = "admin"
    }

    if err := createUser(email, string(hashed), role); err != nil {
        c.String(http.StatusConflict, fmt.Sprintf("회원가입 오류: %v", err))
        return
    }

    // 첫 회원이 아니면 어드민에게 알림 (예시 로그)
    if role != "admin" {
        log.Printf("[이메일 발송] 신규 회원(%s) 가입! 어드민 권한 부여 필요.\n", email)
    }

    // 회원가입 후 자동 로그인
    sess := sessions.Default(c)
    sess.Set("user_email", email)
    sess.Save()

    c.Redirect(http.StatusFound, "/console")
}

// ======================================================
// 4. 인증/인가 미들웨어
// ======================================================

func AuthRequired() gin.HandlerFunc {
    return func(c *gin.Context) {
        sess := sessions.Default(c)
        if sess.Get("user_email") == nil {
            c.Redirect(http.StatusFound, "/")
            c.Abort()
            return
        }
        c.Next()
    }
}

func doLogout(c *gin.Context) {
    sess := sessions.Default(c)
    sess.Clear()
    sess.Save()
    c.Redirect(http.StatusFound, "/")
}

func currentUser(c *gin.Context) *User {
    sess := sessions.Default(c)
    email := sess.Get("user_email")
    if email == nil {
        return nil
    }
    e, ok := email.(string)
    if !ok {
        return nil
    }
    return users[e]
}

func isAdmin(u *User) bool {
    return u != nil && u.Role == "admin"
}


// 전역 변수에 저장
var composeCommand string

// detectDockerComposeCommand: v2("docker compose") 또는 v1("docker-compose") 중 사용 가능 여부 감지
func detectDockerComposeCommand() (string, error) {
    // 1) "docker compose version" 시도
    if err := exec.Command("docker", "compose", "version").Run(); err == nil {
        // 성공하면 v2 명령
        return "docker compose", nil
    }
    // 2) "docker-compose version" 시도
    if err := exec.Command("docker-compose", "version").Run(); err == nil {
        // 성공하면 v1 명령
        return "docker-compose", nil
    }

    // 둘 다 실패하면 에러
    return "", fmt.Errorf("docker-compose 명령을 찾을 수 없습니다. docker compose / docker-compose 모두 불가")
}


// ======================================================
// 5. 도커 컴포즈 웹콘솔 (/console)
// ======================================================

func consolePage(c *gin.Context) {
    user := currentUser(c)
    if user == nil {
        c.Redirect(http.StatusFound, "/")
        return
    }
    data := gin.H{
        "Email":   user.Email,
        "Role":    user.Role,
        "IsAdmin": isAdmin(user),
    }
    c.HTML(http.StatusOK, "console.html", data)
}

// 디렉토리 목록 (AJAX)
func listDirectoriesAPI(c *gin.Context) {
    dirs, err := ioutil.ReadDir(baseDir)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    var result []string
    for _, d := range dirs {
        if d.IsDir() {
            result = append(result, d.Name())
        }
    }
    c.JSON(http.StatusOK, result)
}

// 파일 목록 (AJAX)
func listFilesAPI(c *gin.Context) {
    dir := c.Query("dir")
    if dir == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "dir 파라미터 필요"})
        return
    }
    fullDir := filepath.Join(baseDir, dir)
    infos, err := ioutil.ReadDir(fullDir)
    if err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
        return
    }
    var files []string
    for _, f := range infos {
        if !f.IsDir() {
            files = append(files, filepath.Join(dir, f.Name()))
        }
    }
    c.JSON(http.StatusOK, files)
}

// 파일 내용 가져오기
func getFileContentAPI(c *gin.Context) {
    p := c.Query("path")
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fullPath := filepath.Join(baseDir, p)
    data, err := ioutil.ReadFile(fullPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("파일 읽기 오류: %v", err))
        return
    }
    c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
}

// 파일 저장 (백업 후 저장, 필요 시 도커 재시작)
func saveFileAPI(c *gin.Context) {
    p := c.PostForm("path")
    content := c.PostForm("content")
    doRestart := c.PostForm("restart")
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fullPath := filepath.Join(baseDir, p)

    // 저장 전 백업
    if err := backupFile(fullPath); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 실패: %v", err))
        return
    }
    // 새 내용 저장
    if err := ioutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("저장 실패: %v", err))
        return
    }

    msg := "저장 완료!"
    // 도커 재시작
    if doRestart == "1" {
        out, err := dockerComposeRestart(fullPath)
        if err != nil {
            msg += fmt.Sprintf("\n도커 재시작 오류: %v\n출력:%s", err, out)
        } else {
            msg += "\n도커 재시작 완료!"
        }
    }
    c.String(http.StatusOK, msg)
}

// 도커 재시작
func restartDockerAPI(c *gin.Context) {
    p := c.PostForm("path")
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fullPath := filepath.Join(baseDir, p)
    out, err := dockerComposeRestart(fullPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력:%s", err, out))
        return
    }
    c.String(http.StatusOK, "도커 재시작 완료!")
}

// ------------------------------------------------------
// 6. 백업 로직 (각 디렉토리에 backups/ 폴더)
// ------------------------------------------------------

func backupFile(filePath string) error {
    // 원본 파일 읽기
    data, err := ioutil.ReadFile(filePath)
    if err != nil {
        return err
    }

    // (1) 파일이 있는 디렉토리 내 "backups" 폴더 생성 (없으면)
    dirName := filepath.Dir(filePath)
    localBackupDir := filepath.Join(dirName, "backups")
    if _, err := os.Stat(localBackupDir); os.IsNotExist(err) {
        if err := os.MkdirAll(localBackupDir, 0755); err != nil {
            return fmt.Errorf("백업 디렉토리 생성 오류: %v", err)
        }
    }

    // (2) 백업 파일 이름: [기존파일명_yyyyMMdd_HHmmss.확장자]
    fileName := filepath.Base(filePath)
    ext := filepath.Ext(fileName)
    base := fileName[0 : len(fileName)-len(ext)]
    timestamp := time.Now().Format("20060102_150405")
    backupName := fmt.Sprintf("%s_%s%s", base, timestamp, ext)
    backupPath := filepath.Join(localBackupDir, backupName)

    // (3) 백업 파일로 저장
    if err := ioutil.WriteFile(backupPath, data, 0644); err != nil {
        return fmt.Errorf("백업 파일 저장 오류: %v", err)
    }

    // (4) 백업 정리 (최대 20개)
    return pruneBackups(localBackupDir, base, ext, 20)
}

// pruneBackups: localBackupDir에 있는 특정 파일(base+확장자)의 백업이 20개 초과하면 오래된 것부터 삭제
func pruneBackups(localBackupDir, base, ext string, max int) error {
    files, err := ioutil.ReadDir(localBackupDir)
    if err != nil {
        return err
    }

    var backups []os.FileInfo
    prefix := base + "_"
    for _, f := range files {
        if !f.IsDir() && strings.HasPrefix(f.Name(), prefix) && strings.HasSuffix(f.Name(), ext) {
            backups = append(backups, f)
        }
    }

    // 수정시간 기준 오름차순 정렬(가장 오래된 것이 맨 앞)
    sort.Slice(backups, func(i, j int) bool {
        return backups[i].ModTime().Before(backups[j].ModTime())
    })

    // max 개수 초과분 삭제
    if len(backups) > max {
        for _, f := range backups[:len(backups)-max] {
            os.Remove(filepath.Join(localBackupDir, f.Name()))
        }
    }
    return nil
}

// ------------------------------------------------------
// 7. 백업 목록, 다운로드, 롤백
// ------------------------------------------------------

// 백업 목록
func listBackupsAPI(c *gin.Context) {
    p := c.Query("path") // 예: testtt/aaa.yml
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fullPath := filepath.Join(baseDir, p)

    // 파일명에서 base / ext 추출
    fileName := filepath.Base(fullPath)
    ext := filepath.Ext(fileName)
    base := fileName[0 : len(fileName)-len(ext)]

    // 해당 파일 디렉토리의 backups 폴더
    dirName := filepath.Dir(fullPath)
    localBackupDir := filepath.Join(dirName, "backups")

    // backups 폴더 없으면 목록 없음
    if _, err := os.Stat(localBackupDir); os.IsNotExist(err) {
        c.Data(http.StatusOK, "text/html; charset=utf-8", []byte("<h3>백업 목록</h3><p>백업 없음</p>"))
        return
    }

    files, err := ioutil.ReadDir(localBackupDir)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 디렉토리 오류: %v", err))
        return
    }

    var sb strings.Builder
    sb.WriteString("<h3>백업 목록</h3><ul>")
    for _, f := range files {
        if !f.IsDir() && strings.HasPrefix(f.Name(), base+"_") {
            // 다운로드/롤백 링크를 만들어서 반환
            sb.WriteString(fmt.Sprintf(`
<li>%s
  <a href="/console/api/backup/download?backupfile=%s&target=%s" target="_blank">[다운로드]</a>
  <button onclick="rollbackBackup('%s')">롤백</button>
</li>
`, f.Name(), f.Name(), p, f.Name()))
        }
    }
    sb.WriteString("</ul>")
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

// 백업 다운로드
func downloadBackupAPI(c *gin.Context) {
    bf := c.Query("backupfile") // 예: aaa_20250301_120000.yml
    target := c.Query("target") // 예: testtt/aaa.yml
    if bf == "" || target == "" {
        c.String(http.StatusBadRequest, "backupfile, target 모두 필요")
        return
    }

    // target 파일의 디렉토리에 있는 backups 폴더
    fullPath := filepath.Join(baseDir, target)
    dirName := filepath.Dir(fullPath)
    localBackupDir := filepath.Join(dirName, "backups")
    backupPath := filepath.Join(localBackupDir, bf)

    f, err := os.Open(backupPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 파일 열기 실패: %v", err))
        return
    }
    defer f.Close()

    c.Header("Content-Disposition", "attachment; filename="+bf)
    c.Header("Content-Type", "application/octet-stream")
    io.Copy(c.Writer, f)
}

func rollbackFileAPI(c *gin.Context) {
    bf := c.PostForm("backupfile")  // ex) aaa_20250301_235959.yml
    target := c.PostForm("target")  // ex) testtt/aaa.yml
    if bf == "" || target == "" {
        c.String(http.StatusBadRequest, "backupfile, target 모두 필요")
        return
    }

    // target 파일 경로 및 backupPath
    fullPath := filepath.Join(baseDir, target)
    dirName := filepath.Dir(fullPath)
    localBackupDir := filepath.Join(dirName, "backups")
    backupPath := filepath.Join(localBackupDir, bf)

    // ========== 1) 롤백 전, 현재 파일을 새로 백업해 둔다. ==========
    if err := backupFile(fullPath); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("롤백 실패(현재 파일 백업 중 오류): %v", err))
        return
    }

    // ========== 2) 과거 백업본(rollback 대상)을 읽어서, 현재 파일에 덮어쓰기 ==========
    data, err := ioutil.ReadFile(backupPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 파일 읽기 실패: %v", err))
        return
    }
    if err := ioutil.WriteFile(fullPath, data, 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("롤백 실패(덮어쓰기 오류): %v", err))
        return
    }

    // ========== 3) Docker Compose 재시작 ==========
    out, err := dockerComposeRestart(fullPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력:%s", err, out))
        return
    }

    c.String(http.StatusOK, "롤백(현재 상태 백업 후 과거 버전 복원) 및 도커 재시작 완료!")
}

// ------------------------------------------------------
// 8. 디렉토리/파일 생성
// ------------------------------------------------------

func createDirectoryAPI(c *gin.Context) {
    dirname := c.PostForm("dirname")
    if dirname == "" {
        c.String(http.StatusBadRequest, "dirname 필요")
        return
    }
    targetPath := filepath.Join(baseDir, dirname)
    if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
        c.String(http.StatusBadRequest, "이미 존재하는 디렉토리")
        return
    }
    if err := os.Mkdir(targetPath, 0755); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("디렉토리 생성 오류: %v", err))
        return
    }
    c.String(http.StatusOK, "디렉토리 생성 완료!")
}

func createFileAPI(c *gin.Context) {
    dir := c.PostForm("dir")
    filename := c.PostForm("filename")
    if dir == "" || filename == "" {
        c.String(http.StatusBadRequest, "dir, filename 필요")
        return
    }
    fullDir := filepath.Join(baseDir, dir)
    target := filepath.Join(fullDir, filename)
    if _, err := os.Stat(target); !os.IsNotExist(err) {
        c.String(http.StatusBadRequest, "이미 존재하는 파일")
        return
    }
    // 빈 파일 생성
    if err := ioutil.WriteFile(target, []byte(""), 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("파일 생성 오류: %v", err))
        return
    }
    c.String(http.StatusOK, fmt.Sprintf("파일 생성 완료: %s", filename))
}

// ------------------------------------------------------
// 9. 어드민 페이지 (/console/admin)
// ------------------------------------------------------

func adminPage(c *gin.Context) {
    var userList []map[string]string
    for _, u := range users {
        userList = append(userList, map[string]string{
            "Email": u.Email,
            "Role":  u.Role,
        })
    }
    c.HTML(http.StatusOK, "admin.html", gin.H{
        "Users": userList,
    })
}

func updateUserRole(c *gin.Context) {
    email := c.PostForm("email")
    role := c.PostForm("role")
    if email == "" || (role != "admin" && role != "none") {
        c.String(http.StatusBadRequest, "잘못된 요청")
        return
    }
    u, ok := users[email]
    if !ok {
        c.String(http.StatusBadRequest, "사용자를 찾을 수 없습니다.")
        return
    }
    u.Role = role
    saveAccounts()
    c.String(http.StatusOK, "권한이 업데이트되었습니다. <a href='/console/admin'>돌아가기</a>")
}

func adminOnly(fn gin.HandlerFunc) gin.HandlerFunc {
    return func(c *gin.Context) {
        u := currentUser(c)
        if u == nil || u.Role != "admin" {
            // 어드민이 아닌 경우
            msg := "해당 콘솔의 사용을 위해서는 관리자 권한이 필요합니다.\n" +
                   "어드민 이메일: " + firstRegisteredUserEmail
            c.String(http.StatusForbidden, msg)
            c.Abort()
            return
        }
        // 어드민이면 정상 진입
        fn(c)
    }
}




// ------------------------------------------------------
// 10. Docker Compose 재시작 로직
// ------------------------------------------------------

// dockerComposeRestart: "docker-compose -f [파일] down; sleep 2; up -d" 실행
func dockerComposeRestart(filePath string) (string, error) {
    if composeCommand == "" {
        return "", fmt.Errorf("docker compose 명령이 감지되지 않았습니다.")
    }

    dir := filepath.Dir(filePath)
    fileBase := filepath.Base(filePath)

    // 예) composeCommand = "docker compose"
    // -> parts[0] = "docker", parts[1] = "compose"
    parts := strings.Split(composeCommand, " ")
    // down 명령
    argsDown := append(parts[1:], "-f", fileBase, "down")
    cmdDown := exec.Command(parts[0], argsDown...)
    cmdDown.Dir = dir
    outDown, errDown := cmdDown.CombinedOutput()
    if errDown != nil {
        return string(outDown), errDown
    }

    time.Sleep(2 * time.Second)

    // up -d 명령
    argsUp := append(parts[1:], "-f", fileBase, "up", "-d")
    cmdUp := exec.Command(parts[0], argsUp...)
    cmdUp.Dir = dir
    outUp, errUp := cmdUp.CombinedOutput()

    return string(outDown) + "\n" + string(outUp), errUp
}



// ------------------------------------------------------
// 11. 서버 실행 함수 (runServer)
// ------------------------------------------------------

func runServer() {
    // .env 로드
    if err := godotenv.Load(".env"); err != nil {
        log.Printf("[경고] .env 파일을 찾지 못했거나 로드 오류: %v", err)
    }

    // 포트 설정
    portStr := os.Getenv("port")
    if portStr == "" {
        portStr = "15500"
    }
    serverPort := ":" + portStr

    // 사용자 로드
    if err := loadAccounts(); err != nil {
        log.Println("사용자 정보 로드 오류:", err)
    }

    // 디렉토리 준비
    if _, err := os.Stat(baseDir); os.IsNotExist(err) {
        os.Mkdir(baseDir, 0755)
    }


    // ★ docker compose vs docker-compose 명령 감지 ★
    cmd, err := detectDockerComposeCommand()
    if err != nil {
        log.Fatalf("[에러] Docker Compose 명령 감지 실패: %v\n", err)
    }
    composeCommand = cmd
    log.Printf("Docker Compose 명령어로 '%s' 를 사용합니다.\n", composeCommand)


    // Gin 설정
    r := gin.Default()
    r.LoadHTMLGlob("templates/*.html")

    // 세션
    store := cookie.NewStore([]byte("secret"))
    r.Use(sessions.Sessions("mysession", store))

    // 로그인 불필요 라우트
    r.GET("/", landingPage)
    r.POST("/login", doLogin)
    r.GET("/register", showRegister)
    r.POST("/register", doRegister)

    // 로그인 필요한 라우트
    auth := r.Group("/")
    auth.Use(AuthRequired()) // 로그인 필요
    {
       // ★ adminOnly 추가 ★
       auth.GET("/console", adminOnly(consolePage))

       // 디렉토리/파일 관련 API
       auth.GET("/console/api/dir", adminOnly(listDirectoriesAPI))
       auth.GET("/console/api/files", adminOnly(listFilesAPI))
       auth.GET("/console/api/file", adminOnly(getFileContentAPI))
       auth.POST("/console/api/file", adminOnly(saveFileAPI))
       auth.POST("/console/api/restart", adminOnly(restartDockerAPI))
       auth.GET("/console/api/backups", adminOnly(listBackupsAPI))
       auth.GET("/console/api/backup/download", adminOnly(downloadBackupAPI))
       auth.POST("/console/api/backup/rollback", adminOnly(rollbackFileAPI))
       auth.POST("/console/api/dir/create", adminOnly(createDirectoryAPI))
       auth.POST("/console/api/file/create", adminOnly(createFileAPI))

       // 어드민 페이지도 당연히 adminOnly
       auth.GET("/console/admin", adminOnly(adminPage))
       auth.POST("/console/admin/role", adminOnly(updateUserRole))

       // 로그아웃 등은 adminOnly 아닙니다 (모두 가능)
       auth.GET("/logout", doLogout)
    }

    log.Printf("서버가 포트 %s 로 시작됩니다.\n", serverPort)
    if err := r.Run(serverPort); err != nil {
        log.Fatalf("서버 실행 중 오류: %v", err)
    }
}

// ------------------------------------------------------
// 12. 데몬/런타임 제어 (start|stop|run)
// ------------------------------------------------------

func main() {
    if len(os.Args) < 2 {
        fmt.Println("사용법: dc_webconsole <start|stop|run>")
        return
    }
    cmd := os.Args[1]

    switch cmd {
    case "start":
        startDaemon()
    case "stop":
        stopDaemon()
    case "run":
        // 포그라운드 실행
        runServer()
    default:
        fmt.Printf("알 수 없는 명령어: %s\n", cmd)
        fmt.Println("사용법: dc_webconsole <start|stop|run>")
    }
}

// startDaemon: 백그라운드(데몬)로 서버 실행
func startDaemon() {
    // 중복 실행 방지
    if _, err := os.Stat(pidFile); err == nil {
        fmt.Printf("이미 '%s' 파일이 존재합니다. 서버가 실행 중인지 확인해주세요.\n", pidFile)
        return
    }

    // 현재 실행파일 경로
    exePath, err := os.Executable()
    if err != nil {
        fmt.Println("실행파일 경로를 가져올 수 없습니다.", err)
        return
    }

    // "run" 모드로 자기 자신을 백그라운드 실행
    cmd := exec.Command(exePath, "run")

    // 로그 파일
    f, err := os.OpenFile(logFile, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
    if err != nil {
        fmt.Println("로그 파일을 열 수 없습니다.", err)
        return
    }
    defer f.Close()

    cmd.Stdout = f
    cmd.Stderr = f

    // 비동기 시작
    if err := cmd.Start(); err != nil {
        fmt.Println("서버 데몬 실행 실패:", err)
        return
    }

    // PID 파일 기록
    pid := cmd.Process.Pid
    if err := ioutil.WriteFile(pidFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
        fmt.Println("PID 파일 생성 실패:", err)
        return
    }

    fmt.Printf("서버가 데몬으로 시작되었습니다. (PID: %d)\n", pid)
    fmt.Printf("로그: %s\n", logFile)
}

// stopDaemon: pidFile을 읽어 프로세스 종료
func stopDaemon() {
    data, err := ioutil.ReadFile(pidFile)
    if err != nil {
        fmt.Printf("PID 파일('%s')을 읽을 수 없습니다: %v\n", pidFile, err)
        return
    }
    pidStr := strings.TrimSpace(string(data))
    pid, err := strconv.Atoi(pidStr)
    if err != nil {
        fmt.Println("PID 파일이 올바르지 않습니다:", err)
        return
    }

    proc, err := os.FindProcess(pid)
    if err != nil {
        fmt.Printf("PID %d 프로세스를 찾을 수 없습니다: %v\n", pid, err)
        return
    }

    // 프로세스 종료
    if err := proc.Kill(); err != nil {
        fmt.Printf("PID %d 프로세스를 종료하는 중 오류: %v\n", pid, err)
        return
    }

    // 종료 후 pidFile 삭제
    os.Remove(pidFile)

    fmt.Printf("서버가 중지되었습니다. (PID: %d)\n", pid)
}

