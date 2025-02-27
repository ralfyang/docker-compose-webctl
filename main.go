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
    "strings"
    "time"

    "github.com/gin-contrib/sessions"
    "github.com/gin-contrib/sessions/cookie"
    "github.com/gin-gonic/gin"
    "golang.org/x/crypto/bcrypt"
)

// ------------------------------
// 1. 전역 설정
// ------------------------------

type User struct {
    Email    string
    Password string // bcrypt 해시
    Role     string // "admin" 또는 "none"
}

var users = make(map[string]*User)

const accountFile = ".account"

// docker-compose-list 디렉토리
var baseDir = "./docker-compose-list"

// 백업 파일 디렉토리 (여기서는 전역 사용)
var backupDir = "./backups"

const serverPort = ":15500"

func main() {
    // 사용자 로드
    if err := loadAccounts(); err != nil {
        log.Println("사용자 정보 로드 오류:", err)
    }

    // 디렉토리 준비
    if _, err := os.Stat(baseDir); os.IsNotExist(err) {
        os.Mkdir(baseDir, 0755)
    }
    if _, err := os.Stat(backupDir); os.IsNotExist(err) {
        os.Mkdir(backupDir, 0755)
    }

    // Gin 설정
    r := gin.Default()

    // 템플릿 로딩: templates/*.html
    r.LoadHTMLGlob("templates/*.html")

    // 세션
    store := cookie.NewStore([]byte("secret"))
    r.Use(sessions.Sessions("mysession", store))

    // (A) 로그인 필요 없는 라우트
    r.GET("/", landingPage)        // 랜딩 페이지 (로그인 폼 + 회원가입 버튼)
    r.POST("/login", doLogin)      // 로그인 처리
    r.GET("/register", showRegister)
    r.POST("/register", doRegister)

    // (B) 로그인 필요한 라우트
    auth := r.Group("/")
    auth.Use(AuthRequired())
    {
        // 웹콘솔 페이지
        auth.GET("/console", consolePage)
        // 디렉토리/파일 관련 AJAX
        auth.GET("/console/api/dir", listDirectoriesAPI)
        auth.GET("/console/api/files", listFilesAPI)
        auth.GET("/console/api/file", getFileContentAPI)
        auth.POST("/console/api/file", saveFileAPI)
        auth.POST("/console/api/restart", restartDockerAPI)
        auth.GET("/console/api/backups", listBackupsAPI)
        auth.GET("/console/api/backup/download", downloadBackupAPI)
        auth.POST("/console/api/backup/rollback", rollbackFileAPI)
        auth.POST("/console/api/dir/create", createDirectoryAPI)
        auth.POST("/console/api/file/create", createFileAPI)

        // 어드민 페이지
        auth.GET("/console/admin", adminOnly(adminPage))
        auth.POST("/console/admin/role", adminOnly(updateUserRole))

        // 로그아웃
        auth.GET("/logout", doLogout)
    }

    log.Printf("서버가 포트 %s 로 시작됩니다.\n", serverPort)
    r.Run(serverPort)
}

// ------------------------------
// 2. .account 로드/저장
// ------------------------------

func loadAccounts() error {
    f, err := os.Open(accountFile)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    defer f.Close()

    scanner := bufio.NewScanner(f)
    for scanner.Scan() {
        line := scanner.Text()
        fields := strings.Split(line, ",")
        if len(fields) < 3 {
            continue
        }
        email := fields[0]
        password := fields[1]
        role := fields[2]
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

// ------------------------------
// 3. Landing Page & 로그인/회원가입
// ------------------------------

// 랜딩 페이지
func landingPage(c *gin.Context) {
    // templates/landing.html 렌더링
    c.HTML(http.StatusOK, "landing.html", nil)
}

// 로그인 처리
func doLogin(c *gin.Context) {
    email := c.PostForm("email")
    pw := c.PostForm("password")

    user, ok := users[email]
    if !ok {
        c.String(http.StatusUnauthorized, "등록되지 않은 이메일입니다.")
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(pw)); err != nil {
        c.String(http.StatusUnauthorized, "비밀번호가 일치하지 않습니다.")
        return
    }

    sess := sessions.Default(c)
    sess.Set("user_email", email)
    sess.Save()

    // 로그인 후 /console 로 이동
    c.Redirect(http.StatusFound, "/console")
}

// 회원가입 폼
func showRegister(c *gin.Context) {
    // templates/register.html
    c.HTML(http.StatusOK, "register.html", nil)
}

// 회원가입 처리
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

    if role != "admin" {
        log.Printf("[이메일 발송] 신규 회원(%s) 가입! 어드민 권한 부여 필요.\n", email)
    }

    // 회원가입 후 자동 로그인
    sess := sessions.Default(c)
    sess.Set("user_email", email)
    sess.Save()

    // /console 이동
    c.Redirect(http.StatusFound, "/console")
}

// ------------------------------
// 4. 인증/인가 미들웨어
// ------------------------------

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

// 로그아웃
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

// ------------------------------
// 5. 도커 컴포즈 웹콘솔 (/console)
// ------------------------------


func consolePage(c *gin.Context) {
    user := currentUser(c)
    if user == nil {
        c.Redirect(http.StatusFound, "/")
        return
    }
    // templates/console.html 파일에 넘길 데이터
    data := gin.H{
        "Email":   user.Email,
        "Role":    user.Role,
        "IsAdmin": isAdmin(user),
    }
    // 여기서 "console.html"은 templates/console.html 내에 있는 파일 이름
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

// 파일 내용 (AJAX)
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

// 파일 저장 (AJAX)
func saveFileAPI(c *gin.Context) {
    p := c.PostForm("path")
    content := c.PostForm("content")
    doRestart := c.PostForm("restart")
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fullPath := filepath.Join(baseDir, p)
    // 백업
    if err := backupFile(fullPath); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 실패: %v", err))
        return
    }
    // 저장
    if err := ioutil.WriteFile(fullPath, []byte(content), 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("저장 실패: %v", err))
        return
    }
    msg := "저장 완료!"
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

// 도커 재시작 (AJAX)
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

// 백업 목록 (AJAX)
func listBackupsAPI(c *gin.Context) {
    p := c.Query("path")
    if p == "" {
        c.String(http.StatusBadRequest, "path 필요")
        return
    }
    fileName := filepath.Base(p)
    ext := filepath.Ext(fileName)
    base := fileName[0 : len(fileName)-len(ext)]

    files, err := ioutil.ReadDir(backupDir)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 디렉토리 오류: %v", err))
        return
    }
    var sb strings.Builder
    sb.WriteString("<h3>백업 목록</h3><ul>")
    for _, f := range files {
        if !f.IsDir() && strings.HasPrefix(f.Name(), base+"_") {
            sb.WriteString(fmt.Sprintf(`
            <li>%s
              <a href="/console/api/backup/download?backupfile=%s" target="_blank">[다운로드]</a>
              <button onclick="rollbackBackup('%s')">롤백</button>
            </li>`, f.Name(), f.Name(), f.Name()))
        }
    }
    sb.WriteString("</ul>")
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

// 백업 다운로드
func downloadBackupAPI(c *gin.Context) {
    bf := c.Query("backupfile")
    if bf == "" {
        c.String(http.StatusBadRequest, "backupfile 필요")
        return
    }
    backupPath := filepath.Join(backupDir, bf)
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

// 롤백
func rollbackFileAPI(c *gin.Context) {
    bf := c.PostForm("backupfile")
    target := c.PostForm("target")
    if bf == "" || target == "" {
        c.String(http.StatusBadRequest, "backupfile, target 모두 필요")
        return
    }
    backupPath := filepath.Join(backupDir, bf)
    data, err := ioutil.ReadFile(backupPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("백업 파일 읽기 실패: %v", err))
        return
    }
    fullPath := filepath.Join(baseDir, target)
    if err := ioutil.WriteFile(fullPath, data, 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("롤백 실패: %v", err))
        return
    }
    out, err := dockerComposeRestart(fullPath)
    if err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력:%s", err, out))
        return
    }
    c.String(http.StatusOK, "롤백 및 도커 재시작 완료!")
}

// 디렉토리 생성 (AJAX)
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

// 파일 생성 (AJAX)
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
    // 초기 내용 없이 생성
    if err := ioutil.WriteFile(target, []byte(""), 0644); err != nil {
        c.String(http.StatusInternalServerError, fmt.Sprintf("파일 생성 오류: %v", err))
        return
    }
    c.String(http.StatusOK, fmt.Sprintf("파일 생성 완료: %s", filename))
}

// ------------------------------
// 6. 어드민 페이지 (/console/admin)
// ------------------------------

func adminPage(c *gin.Context) {
    // templates/admin.html 에 사용자 목록, 권한 변경
    // 데이터: user 목록
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


// adminOnly: 현재 사용자(세션)에게 admin 권한이 있는지 확인.
// 없다면 403 Forbidden 처리
func adminOnly(fn gin.HandlerFunc) gin.HandlerFunc {
    return func(c *gin.Context) {
        u := currentUser(c)
        if u == nil || u.Role != "admin" {
            c.String(http.StatusForbidden, "어드민 권한이 필요합니다.")
            c.Abort()
            return
        }
        // admin 권한이 있다면 핸들러 실행
        fn(c)
    }
}



// ------------------------------
// 7. 백업 & 도커 재시작
// ------------------------------

func backupFile(filePath string) error {
    data, err := ioutil.ReadFile(filePath)
    if err != nil {
        return err
    }
    fileName := filepath.Base(filePath)
    ext := filepath.Ext(fileName)
    base := fileName[0 : len(fileName)-len(ext)]
    timestamp := time.Now().Format("20060102_150405")
    backupName := fmt.Sprintf("%s_%s%s", base, timestamp, ext)
    backupPath := filepath.Join(backupDir, backupName)
    if err := ioutil.WriteFile(backupPath, data, 0644); err != nil {
        return err
    }
    return pruneBackups(base, ext, 20)
}

func pruneBackups(base, ext string, max int) error {
    files, err := ioutil.ReadDir(backupDir)
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
    sort.Slice(backups, func(i, j int) bool {
        return backups[i].ModTime().Before(backups[j].ModTime())
    })
    if len(backups) > max {
        for _, f := range backups[:len(backups)-max] {
            os.Remove(filepath.Join(backupDir, f.Name()))
        }
    }
    return nil
}

// docker-compose -f [파일] down; sleep 2; up -d
func dockerComposeRestart(filePath string) (string, error) {
    dir := filepath.Dir(filePath)
    fileBase := filepath.Base(filePath)

    // down
    cmdDown := exec.Command("docker-compose", "-f", fileBase, "down")
    cmdDown.Dir = dir
    outDown, errDown := cmdDown.CombinedOutput()
    if errDown != nil {
        return string(outDown), errDown
    }

    time.Sleep(2 * time.Second)

    // up -d
    cmdUp := exec.Command("docker-compose", "-f", fileBase, "up", "-d")
    cmdUp.Dir = dir
    outUp, errUp := cmdUp.CombinedOutput()

    return string(outDown) + "\n" + string(outUp), errUp
}

