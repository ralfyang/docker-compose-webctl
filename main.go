package main

import (
    "bufio"
    "errors"
    "fmt"
    "io"
    "io/ioutil"
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
    Password string
    Role     string
}

var users = make(map[string]*User) // .account 로드
const accountFile = ".account"

// 백업 디렉토리
var backupDir = "backups"

// ** 새롭게 생성/관리할 디렉토리 루트 **
var composeListRoot = "docker-compose-list"

// 서버 포트
const serverPort = ":15500"

func main() {
    // 1) 사용자 로드
    if err := loadAccounts(); err != nil {
        fmt.Println("사용자 정보 로드 오류:", err)
    }

    // 2) 백업 디렉토리 생성
    if _, err := os.Stat(backupDir); os.IsNotExist(err) {
        os.Mkdir(backupDir, 0755)
    }

    // 3) docker-compose-list 디렉토리 생성
    if _, err := os.Stat(composeListRoot); os.IsNotExist(err) {
        os.Mkdir(composeListRoot, 0755)
    }

    // 4) 라우터 설정
    r := gin.Default()

    // 세션 미들웨어
    store := cookie.NewStore([]byte("secret"))
    r.Use(sessions.Sessions("mysession", store))

    // 로그인/회원가입/로그아웃
    r.GET("/", homeHandler)
    r.GET("/register", showRegister)
    r.POST("/register", registerHandler)
    r.GET("/login", showLogin)
    r.POST("/login", loginHandler)
    r.GET("/logout", logoutHandler)

    // 로그인 후
    auth := r.Group("/")
    auth.Use(AuthRequired())
    {
        // 대시보드
        auth.GET("/dashboard", dashboardHandler)

        // 어드민
        auth.GET("/admin", adminOnly(adminDashboard))
        auth.GET("/admin/users", adminOnly(listUsers))
        auth.POST("/admin/users/role", adminOnly(updateUserRole))

        // 디렉토리/파일 생성 관련
        auth.GET("/dir/list", adminOnly(listDirectories))       // docker-compose-list 내 디렉토리 목록
        auth.GET("/dir/create", adminOnly(showCreateDirectory)) // 디렉토리 생성 폼
        auth.POST("/dir/create", adminOnly(createDirectory))    // 디렉토리 생성 처리

        auth.GET("/file/list", adminOnly(listFilesInDirectory))   // 특정 디렉토리의 파일 목록
        auth.GET("/file/create", adminOnly(showCreateFileForm))   // 파일 생성 폼
        auth.POST("/file/create", adminOnly(createFile))          // 파일 생성 처리

        // 파일 편집/백업/롤백/재시작
        auth.GET("/file/edit", adminOnly(showFileEditor))
        auth.POST("/file/edit", adminOnly(saveFile))
        auth.POST("/file/restart", adminOnly(restartDocker))
        auth.GET("/file/revisions", adminOnly(listRevisions))
        auth.POST("/file/rollback", adminOnly(rollbackFile))
        auth.GET("/file/download", adminOnly(downloadBackupFile))
    }

    fmt.Printf("서버가 포트 %s 로 시작되었습니다.\n", serverPort)
    r.Run(serverPort)
}

// ------------------------------
// 2. 공통 HTML 헤더/푸터
// ------------------------------

func htmlHeader(title string) string {
    return `<!DOCTYPE html>
<html>
<head>
<meta charset="utf-8">
<title>` + title + `</title>
<style>
body {
    font-family: Arial, sans-serif;
    background-color: #f9f9f9;
    margin: 0; padding: 0;
}
.container {
    width: 80%;
    margin: 0 auto;
    text-align: center;
    padding: 20px;
}
h1 { color: #333; }
a { color: #0066cc; text-decoration: none; }
a:hover { text-decoration: underline; }
form { margin: 20px auto; }
input[type="text"], input[type="email"], input[type="password"], textarea {
    width: 80%;
    padding: 8px;
    margin: 5px 0;
}
button, input[type="submit"] {
    padding: 8px 16px;
    margin: 5px;
    background-color: #0066cc;
    color: white;
    border: none;
    cursor: pointer;
}
button:hover, input[type="submit"]:hover {
    background-color: #004999;
}
</style>
</head>
<body>
<div class="container">
`
}

func htmlFooter() string {
    return `
</div>
</body>
</html>`
}

// ------------------------------
// 3. 사용자 (.account)
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

func getUserByEmail(email string) *User {
    return users[email]
}

func createUser(email, hashedPassword, role string) error {
    if _, exists := users[email]; exists {
        return errors.New("이미 등록된 이메일입니다")
    }
    users[email] = &User{Email: email, Password: hashedPassword, Role: role}
    return saveAccounts()
}

func updateUserRoleInFile(email, role string) error {
    u := getUserByEmail(email)
    if u == nil {
        return errors.New("사용자를 찾을 수 없습니다")
    }
    u.Role = role
    return saveAccounts()
}

// ------------------------------
// 4. 인증/인가 (회원가입, 로그인 등)
// ------------------------------

func homeHandler(c *gin.Context) {
    content := htmlHeader("홈") + `
    <h1>웹 콘솔 홈</h1>
    <a href="/register">회원가입</a> | <a href="/login">로그인</a>
    ` + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func showRegister(c *gin.Context) {
    content := htmlHeader("회원가입") + `
    <h1>회원가입</h1>
    <form method="POST" action="/register">
        이메일: <input type="email" name="email" required /><br/>
        비밀번호: <input type="password" name="password" required /><br/>
        <input type="submit" value="회원가입" />
    </form>
    ` + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func registerHandler(c *gin.Context) {
    email := c.PostForm("email")
    password := c.PostForm("password")

    if email == "" || password == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("이메일과 비밀번호는 필수입니다."))
        return
    }
    hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte("비밀번호 해싱 오류"))
        return
    }
    role := "none"
    if len(users) == 0 {
        role = "admin"
    }
    if err := createUser(email, string(hashed), role); err != nil {
        c.Data(http.StatusConflict, "text/html; charset=utf-8", []byte(fmt.Sprintf("회원가입 오류: %v", err)))
        return
    }
    if role != "admin" {
        sendApprovalEmail(email)
    }
    msg := `회원가입 성공! <a href='/login'>로그인</a> 하세요.`
    content := htmlHeader("회원가입 성공") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func showLogin(c *gin.Context) {
    content := htmlHeader("로그인") + `
    <h1>로그인</h1>
    <form method="POST" action="/login">
        이메일: <input type="email" name="email" required /><br/>
        비밀번호: <input type="password" name="password" required /><br/>
        <input type="submit" value="로그인" />
    </form>
    ` + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func loginHandler(c *gin.Context) {
    email := c.PostForm("email")
    password := c.PostForm("password")

    user := getUserByEmail(email)
    if user == nil {
        c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte("등록되지 않은 이메일입니다."))
        return
    }
    if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
        c.Data(http.StatusUnauthorized, "text/html; charset=utf-8", []byte("비밀번호가 일치하지 않습니다."))
        return
    }
    session := sessions.Default(c)
    session.Set("user_email", user.Email)
    session.Save()

    msg := `로그인 성공! <a href='/dashboard'>대시보드</a>`
    content := htmlHeader("로그인 성공") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func logoutHandler(c *gin.Context) {
    session := sessions.Default(c)
    session.Clear()
    session.Save()
    c.Redirect(http.StatusFound, "/")
}

func AuthRequired() gin.HandlerFunc {
    return func(c *gin.Context) {
        session := sessions.Default(c)
        userEmail := session.Get("user_email")
        if userEmail == nil {
            c.Redirect(http.StatusFound, "/login")
            c.Abort()
            return
        }
        c.Next()
    }
}

func adminOnly(handler gin.HandlerFunc) gin.HandlerFunc {
    return func(c *gin.Context) {
        user := currentUser(c)
        if user == nil || user.Role != "admin" {
            c.Data(http.StatusForbidden, "text/html; charset=utf-8", []byte("어드민 권한이 필요합니다."))
            c.Abort()
            return
        }
        handler(c)
    }
}

func currentUser(c *gin.Context) *User {
    session := sessions.Default(c)
    email := session.Get("user_email")
    if email == nil {
        return nil
    }
    emailStr, ok := email.(string)
    if !ok {
        return nil
    }
    return getUserByEmail(emailStr)
}

func dashboardHandler(c *gin.Context) {
    user := currentUser(c)
    content := htmlHeader("대시보드") + fmt.Sprintf(`
    <h1>대시보드</h1>
    <p>현재 사용자: %s (권한: %s)</p>
    <p>
        <a href="/logout">로그아웃</a> | 
        <a href="/admin">어드민 페이지</a> | 
        <a href="/dir/list">디렉토리 목록</a>
    </p>
    `, user.Email, user.Role) + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// ------------------------------
// 5. 어드민 페이지
// ------------------------------

func adminDashboard(c *gin.Context) {
    content := htmlHeader("어드민 대시보드") + `
    <h1>어드민 대시보드</h1>
    <ul>
        <li><a href="/admin/users">사용자 관리</a></li>
        <li><a href="/dir/list">디렉토리 목록</a></li>
    </ul>
    <a href="/dashboard">대시보드</a>
    ` + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func listUsers(c *gin.Context) {
    var sb strings.Builder
    sb.WriteString("<h1>사용자 목록</h1><ul>")
    for _, u := range users {
        sb.WriteString(fmt.Sprintf(`
        <li>
            Email: %s, Role: %s
            <form style="display:inline;" method="POST" action="/admin/users/role">
                <input type="hidden" name="email" value="%s"/>
                <select name="role">
                    <option value="none" %s>권한없음</option>
                    <option value="admin" %s>어드민</option>
                </select>
                <input type="submit" value="변경" />
            </form>
        </li>`,
            u.Email, u.Role, u.Email,
            selectedIf(u.Role == "none"),
            selectedIf(u.Role == "admin")))
    }
    sb.WriteString("</ul><a href='/admin'>어드민 대시보드</a>")
    content := htmlHeader("사용자 목록") + sb.String() + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func updateUserRole(c *gin.Context) {
    email := c.PostForm("email")
    role := c.PostForm("role")
    if email == "" || (role != "admin" && role != "none") {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("잘못된 요청"))
        return
    }
    if err := updateUserRoleInFile(email, role); err != nil {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte(fmt.Sprintf("권한 업데이트 실패: %v", err)))
        return
    }
    msg := "권한이 업데이트되었습니다. <a href='/admin/users'>돌아가기</a>"
    content := htmlHeader("업데이트 성공") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func selectedIf(cond bool) string {
    if cond {
        return "selected"
    }
    return ""
}

// ------------------------------
// 6. 디렉토리/파일 생성
// ------------------------------

func listDirectories(c *gin.Context) {
    // docker-compose-list 아래의 서브 디렉토리 목록 표시
    dirs, err := ioutil.ReadDir(composeListRoot)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("디렉토리 조회 오류: %v", err)))
        return
    }
    var sb strings.Builder
    sb.WriteString("<h1>디렉토리 목록</h1><ul>")
    for _, d := range dirs {
        if d.IsDir() {
            sb.WriteString(fmt.Sprintf(`
            <li>%s 
                <a href="/file/list?dir=%s">[파일 목록 보기]</a>
            </li>`, d.Name(), d.Name()))
        }
    }
    sb.WriteString("</ul>")
    sb.WriteString(`<a href="/dir/create">디렉토리 생성</a> | <a href="/admin">어드민 페이지</a>`)
    content := htmlHeader("디렉토리 목록") + sb.String() + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func showCreateDirectory(c *gin.Context) {
    html := `
    <h1>디렉토리 생성</h1>
    <form method="POST" action="/dir/create">
        <label>디렉토리명:</label><br/>
        <input type="text" name="dirname" required /><br/>
        <input type="submit" value="생성" />
    </form>
    <a href="/dir/list">돌아가기</a>`
    content := htmlHeader("디렉토리 생성") + html + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

func createDirectory(c *gin.Context) {
    dirname := c.PostForm("dirname")
    if dirname == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("디렉토리명이 비어있습니다."))
        return
    }
    targetPath := filepath.Join(composeListRoot, dirname)
    if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("이미 존재하는 디렉토리입니다."))
        return
    }
    if err := os.Mkdir(targetPath, 0755); err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("디렉토리 생성 오류: %v", err)))
        return
    }
    msg := fmt.Sprintf("디렉토리 생성 완료: %s <br/><a href='/dir/list'>디렉토리 목록</a>", dirname)
    content := htmlHeader("생성 완료") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 파일 목록
func listFilesInDirectory(c *gin.Context) {
    dir := c.Query("dir")
    if dir == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("dir 파라미터가 없습니다."))
        return
    }
    fullDir := filepath.Join(composeListRoot, dir)
    infos, err := ioutil.ReadDir(fullDir)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("디렉토리 열기 오류: %v", err)))
        return
    }
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("<h1>[%s] 파일 목록</h1><ul>", dir))
    for _, f := range infos {
        if !f.IsDir() {
            filePath := filepath.Join(fullDir, f.Name())
            sb.WriteString(fmt.Sprintf(`
            <li>%s 
                <a href="/file/edit?path=%s">[편집]</a> |
                <form style="display:inline;" method="POST" action="/file/restart">
                    <input type="hidden" name="path" value="%s"/>
                    <input type="submit" value="재시작" />
                </form> |
                <a href="/file/revisions?path=%s">[백업/리비전 보기]</a>
            </li>`, f.Name(), filePath, filePath, filePath))
        }
    }
    sb.WriteString("</ul>")
    sb.WriteString(fmt.Sprintf(`<a href="/file/create?dir=%s">파일 생성</a> | <a href="/dir/list">디렉토리 목록</a>`, dir))
    content := htmlHeader("파일 목록") + sb.String() + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 파일 생성 폼
func showCreateFileForm(c *gin.Context) {
    dir := c.Query("dir")
    if dir == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("dir 파라미터가 없습니다."))
        return
    }
    html := fmt.Sprintf(`
    <h1>파일 생성</h1>
    <p>디렉토리: %s</p>
    <form method="POST" action="/file/create">
        <input type="hidden" name="dir" value="%s"/>
        <label>파일명 (예: docker-compose.yml):</label><br/>
        <input type="text" name="filename" required /><br/>
        <br/>
        <textarea name="content" rows="10" cols="50" placeholder="초기 내용"></textarea><br/>
        <input type="submit" value="생성" />
    </form>
    <a href="/file/list?dir=%s">돌아가기</a>
    `, dir, dir, dir)
    content := htmlHeader("파일 생성") + html + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 파일 생성 처리
func createFile(c *gin.Context) {
    dir := c.PostForm("dir")
    filename := c.PostForm("filename")
    contentText := c.PostForm("content")

    if dir == "" || filename == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("dir 또는 filename이 없습니다."))
        return
    }
    targetPath := filepath.Join(composeListRoot, dir, filename)
    if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("이미 존재하는 파일입니다."))
        return
    }
    if err := ioutil.WriteFile(targetPath, []byte(contentText), 0644); err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("파일 생성 오류: %v", err)))
        return
    }
    msg := fmt.Sprintf("파일 생성 완료: %s <br/><a href='/file/list?dir=%s'>파일 목록</a>", filename, dir)
    content := htmlHeader("생성 완료") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// ------------------------------
// 7. 파일 편집/백업/롤백/재시작
// ------------------------------

// 파일 편집 화면
func showFileEditor(c *gin.Context) {
    path := c.Query("path")
    if path == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("path 파라미터가 없습니다."))
        return
    }
    contentBytes, err := ioutil.ReadFile(path)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("파일 읽기 오류: %v", err)))
        return
    }
    html := fmt.Sprintf(`
    <h1>%s 편집</h1>
    <form method="POST" action="/file/edit">
        <input type="hidden" name="path" value="%s"/>
        <textarea name="content" rows="20" cols="80">%s</textarea><br/>
        <button type="submit" name="action" value="save">저장</button>
        <button type="submit" name="action" value="restart">저장 및 시스템 리스타트</button>
    </form>
    <a href="javascript:history.back()">뒤로가기</a>
    `, path, path, contentBytes)
    contentStr := htmlHeader("편집") + html + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(contentStr))
}

// 파일 저장 처리
func saveFile(c *gin.Context) {
    path := c.PostForm("path")
    action := c.PostForm("action")
    newContent := c.PostForm("content")

    if path == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("path 파라미터가 없습니다."))
        return
    }
    // 백업
    if err := backupFile(path); err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("백업 실패: %v", err)))
        return
    }
    // 저장
    if err := ioutil.WriteFile(path, []byte(newContent), 0644); err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("파일 저장 실패: %v", err)))
        return
    }
    msg := "파일 저장 완료!"
    if action == "restart" {
        out, err := restartDockerSystem(path)
        if err != nil {
            c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("도커 재시작 오류: %v<br>출력: %s", err, out)))
            return
        }
        msg += "<br/>도커 재시작 완료!"
    }
    msg += fmt.Sprintf(` <br/><a href="/file/edit?path=%s">편집 화면으로</a>`, path)
    content := htmlHeader("저장 완료") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 도커 재시작
func restartDocker(c *gin.Context) {
    path := c.PostForm("path")
    if path == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("path 파라미터가 없습니다."))
        return
    }
    out, err := restartDockerSystem(path)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("도커 재시작 오류: %v<br>출력: %s", err, out)))
        return
    }
    msg := `도커 재시작 완료!`
    content := htmlHeader("재시작 완료") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 백업 목록
func listRevisions(c *gin.Context) {
    path := c.Query("path")
    if path == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("path 파라미터가 없습니다."))
        return
    }
    fileName := filepath.Base(path)
    ext := filepath.Ext(fileName)
    base := fileName[0 : len(fileName)-len(ext)]

    files, err := ioutil.ReadDir(backupDir)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("백업 디렉토리 조회 오류: %v", err)))
        return
    }
    var sb strings.Builder
    sb.WriteString(fmt.Sprintf("<h1>[%s] 백업 파일 목록</h1><ul>", path))
    for _, f := range files {
        if !f.IsDir() && strings.HasPrefix(f.Name(), base+"_") {
            // 다운로드 링크: /file/download?backupfile=<name>
            sb.WriteString(fmt.Sprintf(`
            <li>%s 
                <a href="/file/download?backupfile=%s" target="_blank">[다운로드]</a>
                <form style="display:inline;" method="POST" action="/file/rollback">
                    <input type="hidden" name="backupfile" value="%s" />
                    <input type="hidden" name="target" value="%s" />
                    <input type="submit" value="롤백" onclick="return confirm('해당 백업파일로 롤백하시겠습니까?');" />
                </form>
            </li>`, f.Name(), f.Name(), f.Name(), path))
        }
    }
    sb.WriteString("</ul>")
    sb.WriteString(fmt.Sprintf(`<a href="/file/edit?path=%s">편집화면</a>`, path))
    content := htmlHeader("백업 목록") + sb.String() + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 롤백
func rollbackFile(c *gin.Context) {
    backupFileName := c.PostForm("backupfile")
    targetFile := c.PostForm("target")
    if backupFileName == "" || targetFile == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("backupfile 또는 target 파라미터가 없습니다."))
        return
    }
    backupPath := filepath.Join(backupDir, backupFileName)
    data, err := ioutil.ReadFile(backupPath)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("백업 파일 읽기 실패: %v", err)))
        return
    }
    if err := ioutil.WriteFile(targetFile, data, 0644); err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("롤백 실패: %v", err)))
        return
    }
    out, err := restartDockerSystem(targetFile)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("도커 재시작 오류: %v<br>출력: %s", err, out)))
        return
    }
    msg := fmt.Sprintf(`롤백 및 도커 재시작 완료! <br/><a href="/file/revisions?path=%s">백업 목록</a>`, targetFile)
    content := htmlHeader("롤백 완료") + "<p>" + msg + "</p>" + htmlFooter()
    c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(content))
}

// 백업 생성 (최근 20개 유지)
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
    // 백업 파일이 20개를 초과하면 오래된 파일 삭제
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
    // 오래된 순으로 정렬 (ModTime 사용)
    sort.Slice(backups, func(i, j int) bool {
        return backups[i].ModTime().Before(backups[j].ModTime())
    })
    // max 개 초과분 삭제
    if len(backups) > max {
        for _, f := range backups[:len(backups)-max] {
            os.Remove(filepath.Join(backupDir, f.Name()))
        }
    }
    return nil
}

// 도커 재시작
func restartDockerSystem(filePath string) (string, error) {
    cmd := exec.Command("bash", "docker-controller.sh", filePath)
    cmd.Dir = filepath.Dir(filePath)
    out, err := cmd.CombinedOutput()
    return string(out), err
}

// 백업파일 다운로드
func downloadBackupFile(c *gin.Context) {
    backupFileName := c.Query("backupfile")
    if backupFileName == "" {
        c.Data(http.StatusBadRequest, "text/html; charset=utf-8", []byte("backupfile 파라미터가 없습니다."))
        return
    }
    backupPath := filepath.Join(backupDir, backupFileName)
    f, err := os.Open(backupPath)
    if err != nil {
        c.Data(http.StatusInternalServerError, "text/html; charset=utf-8", []byte(fmt.Sprintf("파일 열기 실패: %v", err)))
        return
    }
    defer f.Close()
    c.Header("Content-Disposition", "attachment; filename="+backupFileName)
    c.Header("Content-Type", "application/octet-stream")
    io.Copy(c.Writer, f)
}

// ------------------------------
// 8. 부가 함수
// ------------------------------

func sendApprovalEmail(newUserEmail string) {
    fmt.Printf("[이메일 발송] 신규 회원(%s) 가입! 기존 어드민에게 승인 요청\n", newUserEmail)
}

