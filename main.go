package main

import (
	"bufio"
	"bytes"
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

// ---------------------------------------------------
// 1. 전역 설정
// ---------------------------------------------------

type User struct {
	Email    string
	Password string // bcrypt 해시
	Role     string // "admin" 또는 "none"
}

var users = make(map[string]*User)

// .account 파일
const accountFile = ".account"

// docker-compose-list 디렉토리 (웹 콘솔에서 허용할 디렉토리 루트)
var baseDir = "./docker-compose-list"

// 백업 디렉토리
var backupDir = "./backups"

// 서버 포트
const serverPort = ":15500"

func main() {
	// 1) 사용자 로드
	if err := loadAccounts(); err != nil {
		log.Println("사용자 정보 로드 오류:", err)
	}

	// 2) 디렉토리 준비
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		os.Mkdir(baseDir, 0755)
	}
	if _, err := os.Stat(backupDir); os.IsNotExist(err) {
		os.Mkdir(backupDir, 0755)
	}

	// 3) Gin 라우터
	r := gin.Default()
	store := cookie.NewStore([]byte("secret"))
	r.Use(sessions.Sessions("mysession", store))

	// (A) 로그인 필요 없는 라우트
	r.GET("/", homeHandler)
	r.GET("/register", showRegister)
	r.POST("/register", registerHandler)
	r.GET("/login", showLogin)
	r.POST("/login", loginHandler)
	r.GET("/logout", logoutHandler)

	// (B) 로그인 필요한 그룹
	auth := r.Group("/")
	auth.Use(AuthRequired())
	{
		// 대시보드
		auth.GET("/dashboard", dashboardHandler)

		// 어드민
		auth.GET("/admin", adminOnly(adminDashboard))
		auth.GET("/admin/users", adminOnly(listUsers))
		auth.POST("/admin/users/role", adminOnly(updateUserRole))

		// 분할 화면 (AJAX)
		auth.GET("/compose/split", adminOnly(showSplitPage))
		auth.GET("/compose/api/list", adminOnly(apiListFiles))
		auth.GET("/compose/api/file", adminOnly(apiGetFileContent))
		auth.POST("/compose/api/file", adminOnly(apiSaveFileContent))
		auth.POST("/compose/api/restart", adminOnly(apiRestartDocker))
		auth.GET("/compose/api/backups", adminOnly(apiListBackups))
		auth.GET("/compose/api/backup/download", adminOnly(apiDownloadBackup))
		auth.POST("/compose/api/backup/rollback", adminOnly(apiRollbackFile))

		// 기존 폼 기반 라우트
		auth.GET("/dir/list", adminOnly(listDirectories))
		auth.GET("/dir/create", adminOnly(showCreateDirectory))
		auth.POST("/dir/create", adminOnly(createDirectory))

		auth.GET("/file/list", adminOnly(listFilesInDirectory))
		auth.GET("/file/create", adminOnly(showCreateFileForm))
		auth.POST("/file/create", adminOnly(createFile))

		auth.GET("/file/edit", adminOnly(showFileEditor))
		auth.POST("/file/edit", adminOnly(saveFile))
		auth.POST("/file/restart", adminOnly(restartDocker))
		auth.GET("/file/revisions", adminOnly(listRevisions))
		auth.POST("/file/rollback", adminOnly(rollbackFile))
		auth.GET("/file/download", adminOnly(downloadBackupFile))
	}

	log.Printf("서버가 포트 %s 로 시작됩니다.\n", serverPort)
	r.Run(serverPort)
}

// ---------------------------------------------------
// 2. .account 파일 로드/저장
// ---------------------------------------------------

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
	if _, ok := users[email]; ok {
		return errors.New("이미 등록된 이메일입니다")
	}
	users[email] = &User{Email: email, Password: hashedPwd, Role: role}
	return saveAccounts()
}

func updateUserRoleInFile(email, role string) error {
	u, ok := users[email]
	if !ok {
		return errors.New("사용자를 찾을 수 없습니다")
	}
	u.Role = role
	return saveAccounts()
}

// ---------------------------------------------------
// 3. 홈/회원가입/로그인/로그아웃
// ---------------------------------------------------

func homeHandler(c *gin.Context) {
	html := `
    <div style="text-align:center; margin:50px auto;">
      <h1>웹 콘솔 홈</h1>
      <p>
        <a href="/register">회원가입</a> |
        <a href="/login">로그인</a>
      </p>
    </div>
    `
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func showRegister(c *gin.Context) {
	html := `
    <div style="text-align:center; margin:50px auto;">
      <h1>회원가입</h1>
      <form method="POST" action="/register" style="display:inline-block;">
        <div style="margin:10px;">
          이메일: <input type="email" name="email" required/>
        </div>
        <div style="margin:10px;">
          비밀번호: <input type="password" name="password" required/>
        </div>
        <div style="margin:10px;">
          <input type="submit" value="회원가입"/>
        </div>
      </form>
    </div>
    `
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func registerHandler(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")
	if email == "" || password == "" {
		c.String(http.StatusBadRequest, "이메일과 비밀번호는 필수입니다.")
		return
	}

	hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.String(http.StatusInternalServerError, "비밀번호 해싱 오류")
		return
	}

	role := "none"
	if len(users) == 0 {
		role = "admin"
	}
	if err := createUser(email, string(hashed), role); err != nil {
		c.String(http.StatusConflict, fmt.Sprintf("회원가입 오류: %v", err))
		return
	}

	if role != "admin" {
		log.Printf("[이메일 발송] 신규 회원(%s) 가입! 기존 어드민에게 승인 요청\n", email)
	}

	msg := `회원가입 성공! <a href='/login'>로그인</a> 하세요.`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(msg))
}

func showLogin(c *gin.Context) {
	html := `
    <div style="text-align:center; margin:50px auto;">
      <h1>로그인</h1>
      <form method="POST" action="/login" style="display:inline-block;">
        <div style="margin:10px;">
          이메일: <input type="email" name="email" required/>
        </div>
        <div style="margin:10px;">
          비밀번호: <input type="password" name="password" required/>
        </div>
        <div style="margin:10px;">
          <input type="submit" value="로그인"/>
        </div>
      </form>
    </div>
    `
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func loginHandler(c *gin.Context) {
	email := c.PostForm("email")
	password := c.PostForm("password")

	u, ok := users[email]
	if !ok {
		c.String(http.StatusUnauthorized, "등록되지 않은 이메일입니다.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		c.String(http.StatusUnauthorized, "비밀번호가 일치하지 않습니다.")
		return
	}

	session := sessions.Default(c)
	session.Set("user_email", email)
	session.Save()

	msg := `로그인 성공! <a href='/dashboard'>대시보드</a>`
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(msg))
}

func logoutHandler(c *gin.Context) {
	session := sessions.Default(c)
	session.Clear()
	session.Save()
	c.Redirect(http.StatusFound, "/")
}

// ---------------------------------------------------
// 4. 로그인/어드민 미들웨어
// ---------------------------------------------------

func AuthRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		sess := sessions.Default(c)
		if sess.Get("user_email") == nil {
			c.Redirect(http.StatusFound, "/login")
			c.Abort()
			return
		}
		c.Next()
	}
}

func adminOnly(fn gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		u := currentUser(c)
		if u == nil || u.Role != "admin" {
			c.String(http.StatusForbidden, "어드민 권한이 필요합니다.")
			c.Abort()
			return
		}
		fn(c)
	}
}

func currentUser(c *gin.Context) *User {
	sess := sessions.Default(c)
	email := sess.Get("user_email")
	if email == nil {
		return nil
	}
	emailStr, ok := email.(string)
	if !ok {
		return nil
	}
	return users[emailStr]
}

// ---------------------------------------------------
// 5. 대시보드 / 어드민
// ---------------------------------------------------

func dashboardHandler(c *gin.Context) {
	u := currentUser(c)
	html := fmt.Sprintf(`
    <div style="text-align:center; margin:50px auto;">
      <h1>대시보드</h1>
      <p>현재 사용자: %s (권한: %s)</p>
      <p>
        <a href="/logout">로그아웃</a> |
        <a href="/admin">어드민 페이지</a> |
        <a href="/compose/split">도커 컴포즈 분할화면</a>
      </p>
    </div>
    `, u.Email, u.Role)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func adminDashboard(c *gin.Context) {
	html := `
    <div style="text-align:center; margin:50px auto;">
      <h1>어드민 대시보드</h1>
      <ul style="list-style:none;">
        <li><a href="/admin/users">사용자 관리</a></li>
        <li><a href="/compose/split">도커 컴포즈 분할화면</a></li>
        <li><a href="/dir/list">디렉토리 목록</a></li>
      </ul>
      <a href="/dashboard">대시보드</a>
    </div>
    `
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}


// selectedIf는 cond가 true일 때 "selected"를 반환, 아니면 "" 반환
func selectedIf(cond bool) string {
    if cond {
        return "selected"
    }
    return ""
}


// 사용자 목록
func listUsers(c *gin.Context) {
	var sb strings.Builder
	sb.WriteString(`<div style="text-align:center; margin:50px auto;">
	<h1>사용자 목록</h1><ul style="list-style:none;">`)
	for _, u := range users {
		sb.WriteString(fmt.Sprintf(`
		<li style="margin:10px;">
		  이메일: %s, 권한: %s
		  <form style="display:inline;" method="POST" action="/admin/users/role">
		    <input type="hidden" name="email" value="%s"/>
		    <select name="role">
		      <option value="none" %s>권한없음</option>
		      <option value="admin" %s>어드민</option>
		    </select>
		    <input type="submit" value="변경"/>
		  </form>
		</li>`, u.Email, u.Role, u.Email, selectedIf(u.Role == "none"), selectedIf(u.Role == "admin")))
	}
	sb.WriteString("</ul><a href='/admin'>어드민 대시보드</a></div>")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

func updateUserRole(c *gin.Context) {
	email := c.PostForm("email")
	role := c.PostForm("role")
	if email == "" || (role != "admin" && role != "none") {
		c.String(http.StatusBadRequest, "잘못된 요청")
		return
	}
	if err := updateUserRoleInFile(email, role); err != nil {
		c.String(http.StatusBadRequest, fmt.Sprintf("권한 업데이트 실패: %v", err))
		return
	}
	c.String(http.StatusOK, "권한이 업데이트되었습니다. <a href='/admin/users'>돌아가기</a>")
}

// ---------------------------------------------------
// 6. 분할화면 (왼쪽 파일 리스트 / 오른쪽 에디터)
// ---------------------------------------------------

func showSplitPage(c *gin.Context) {
	html := `
    <!DOCTYPE html>
    <html>
    <head>
      <meta charset="utf-8"/>
      <title>도커 컴포즈 분할화면</title>
      <style>
      body { margin:0; padding:0; font-family:Arial,sans-serif; }
      .split-container { display:flex; height:100vh; }
      .left-pane { width:30%; border-right:1px solid #ccc; overflow:auto; }
      .right-pane { width:70%; padding:10px; }
      .file-item { padding:5px; cursor:pointer; }
      .file-item:hover { background:#eee; }
      #editor { width:100%; height:400px; }
      </style>
    </head>
    <body>
      <div class="split-container">
        <div class="left-pane">
          <h2 style="text-align:center;">도커 컴포즈 파일 목록</h2>
          <div id="fileList" style="padding:10px;"></div>
        </div>
        <div class="right-pane">
          <h2>파일 내용</h2>
          <textarea id="editor"></textarea><br/>
          <button onclick="saveFile()">저장</button>
          <button onclick="saveAndRestart()">저장 & 리스타트</button>
          <p><button onclick="loadBackups()">백업 목록 보기</button></p>
          <div id="backupList"></div>
        </div>
      </div>
      <script>
      let currentFile = "";

      async function loadFileList() {
        let resp = await fetch("/compose/api/list");
        if(!resp.ok) { alert("파일 목록 로드 실패"); return; }
        let data = await resp.json();
        let fileListDiv = document.getElementById("fileList");
        fileListDiv.innerHTML = "";
        data.forEach(f => {
          let div = document.createElement("div");
          div.className = "file-item";
          div.textContent = f;
          div.onclick = () => loadFileContent(f);
          fileListDiv.appendChild(div);
        });
      }

      async function loadFileContent(filePath) {
        let url = "/compose/api/file?path=" + encodeURIComponent(filePath);
        let resp = await fetch(url);
        if(!resp.ok) { alert("파일 로드 실패"); return; }
        let text = await resp.text();
        currentFile = filePath;
        document.getElementById("editor").value = text;
        document.getElementById("backupList").innerHTML = "";
      }

      async function saveFile(restart=false) {
        if(!currentFile) { alert("파일이 선택되지 않음"); return; }
        let content = document.getElementById("editor").value;
        let form = new FormData();
        form.append("path", currentFile);
        form.append("content", content);
        form.append("restart", restart ? "1" : "0");
        let resp = await fetch("/compose/api/file", { method:"POST", body:form });
        if(resp.ok) {
          let msg = await resp.text();
          alert(msg);
        } else {
          alert("저장 실패");
        }
      }

      function saveAndRestart() {
        saveFile(true);
      }

      async function loadBackups() {
        if(!currentFile) { alert("파일이 선택되지 않음"); return; }
        let url = "/compose/api/backups?path=" + encodeURIComponent(currentFile);
        let resp = await fetch(url);
        if(!resp.ok) { alert("백업 목록 로드 실패"); return; }
        let html = await resp.text();
        document.getElementById("backupList").innerHTML = html;
      }

      async function rollbackBackup(backupFile) {
        if(!confirm("해당 백업으로 롤백?")) return;
        let form = new FormData();
        form.append("backupfile", backupFile);
        form.append("target", currentFile);
        let resp = await fetch("/compose/api/backup/rollback", { method:"POST", body:form });
        if(resp.ok) {
          let text = await resp.text();
          alert(text);
          loadFileContent(currentFile);
        } else {
          alert("롤백 실패");
        }
      }

      window.onload = () => {
        loadFileList();
      };
      </script>
    </body>
    </html>
    `;
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// GET /compose/api/list -> 파일 목록 (JSON)
func apiListFiles(c *gin.Context) {
	var result []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(baseDir, path)
		result = append(result, rel)
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GET /compose/api/file?path=...
func apiGetFileContent(c *gin.Context) {
	p := c.Query("path")
	if p == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	fullPath := filepath.Join(baseDir, p)
	data, err := ioutil.ReadFile(fullPath)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("파일 읽기 실패: %v", err))
		return
	}
	c.Data(http.StatusOK, "text/plain; charset=utf-8", data)
}

// POST /compose/api/file
func apiSaveFileContent(c *gin.Context) {
	p := c.PostForm("path")
	content := c.PostForm("content")
	doRestart := c.PostForm("restart")
	if p == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
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
	var buf bytes.Buffer
	buf.WriteString("저장 완료!")
	if doRestart == "1" {
		out, err := dockerComposeRestart(fullPath)
		if err != nil {
			buf.WriteString(fmt.Sprintf("\n도커 재시작 오류: %v\n출력: %s", err, out))
		} else {
			buf.WriteString("\n도커 재시작 완료!")
		}
	}
	c.String(http.StatusOK, buf.String())
}

// POST /compose/api/restart
func apiRestartDocker(c *gin.Context) {
	p := c.PostForm("path")
	if p == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	fullPath := filepath.Join(baseDir, p)
	out, err := dockerComposeRestart(fullPath)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력: %s", err, out))
		return
	}
	c.String(http.StatusOK, "도커 재시작 완료!")
}

// GET /compose/api/backups?path=...
func apiListBackups(c *gin.Context) {
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
              <a href="/compose/api/backup/download?backupfile=%s" target="_blank">[다운로드]</a>
              <button onclick="rollbackBackup('%s')">롤백</button>
            </li>`, f.Name(), f.Name(), f.Name()))
		}
	}
	sb.WriteString("</ul>")
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

// GET /compose/api/backup/download?backupfile=...
func apiDownloadBackup(c *gin.Context) {
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

// POST /compose/api/backup/rollback
func apiRollbackFile(c *gin.Context) {
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

// ---------------------------------------------------
// 7. 디렉토리/파일 (폼 기반)
// ---------------------------------------------------

func listDirectories(c *gin.Context) {
	dirs, err := ioutil.ReadDir(baseDir)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("디렉토리 조회 오류: %v", err))
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
	sb.WriteString(`<a href="/dir/create">디렉토리 생성</a>`)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

func showCreateDirectory(c *gin.Context) {
	html := `
    <h1>디렉토리 생성</h1>
    <form method="POST" action="/dir/create">
        <label>디렉토리명:</label><br/>
        <input type="text" name="dirname" required/><br/>
        <input type="submit" value="생성"/>
    </form>
    <a href="/dir/list">돌아가기</a>
    `
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func createDirectory(c *gin.Context) {
	dirname := c.PostForm("dirname")
	if dirname == "" {
		c.String(http.StatusBadRequest, "디렉토리명이 비어있습니다.")
		return
	}
	targetPath := filepath.Join(baseDir, dirname)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		c.String(http.StatusBadRequest, "이미 존재하는 디렉토리입니다.")
		return
	}
	if err := os.Mkdir(targetPath, 0755); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("디렉토리 생성 오류: %v", err))
		return
	}
	msg := fmt.Sprintf("디렉토리 생성 완료: %s <br/><a href='/dir/list'>디렉토리 목록</a>", dirname)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(msg))
}

func listFilesInDirectory(c *gin.Context) {
	dir := c.Query("dir")
	if dir == "" {
		c.String(http.StatusBadRequest, "dir 파라미터가 없습니다.")
		return
	}
	fullDir := filepath.Join(baseDir, dir)
	infos, err := ioutil.ReadDir(fullDir)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("디렉토리 열기 오류: %v", err))
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
                    <input type="submit" value="재시작"/>
                </form> |
                <a href="/file/revisions?path=%s">[백업/리비전 보기]</a>
            </li>`, f.Name(), filePath, filePath, filePath))
		}
	}
	sb.WriteString("</ul>")
	sb.WriteString(fmt.Sprintf(`<a href="/file/create?dir=%s">파일 생성</a> | <a href="/dir/list">디렉토리 목록</a>`, dir))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

func showCreateFileForm(c *gin.Context) {
	dir := c.Query("dir")
	if dir == "" {
		c.String(http.StatusBadRequest, "dir 파라미터가 없습니다.")
		return
	}
	html := fmt.Sprintf(`
    <h1>파일 생성</h1>
    <p>디렉토리: %s</p>
    <form method="POST" action="/file/create">
        <input type="hidden" name="dir" value="%s"/>
        <label>파일명 (예: docker-compose.yml):</label><br/>
        <input type="text" name="filename" required/><br/>
        <br/>
        <textarea name="content" rows="10" cols="50" placeholder="초기 내용"></textarea><br/>
        <input type="submit" value="생성"/>
    </form>
    <a href="/file/list?dir=%s">돌아가기</a>
    `, dir, dir, dir)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

func createFile(c *gin.Context) {
	dir := c.PostForm("dir")
	filename := c.PostForm("filename")
	contentText := c.PostForm("content")

	if dir == "" || filename == "" {
		c.String(http.StatusBadRequest, "dir 또는 filename이 없습니다.")
		return
	}
	targetPath := filepath.Join(baseDir, dir, filename)
	if _, err := os.Stat(targetPath); !os.IsNotExist(err) {
		c.String(http.StatusBadRequest, "이미 존재하는 파일입니다.")
		return
	}
	if err := ioutil.WriteFile(targetPath, []byte(contentText), 0644); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("파일 생성 오류: %v", err))
		return
	}
	msg := fmt.Sprintf("파일 생성 완료: %s <br/><a href='/file/list?dir=%s'>파일 목록</a>", filename, dir)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(msg))
}

// 파일 편집
func showFileEditor(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	data, err := ioutil.ReadFile(path)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("파일 읽기 오류: %v", err))
		return
	}
	html := fmt.Sprintf(`
    <h1>%s 편집</h1>
    <form method="POST" action="/file/edit">
        <input type="hidden" name="path" value="%s"/>
        <textarea name="content" rows="20" cols="80">%s</textarea><br/>
        <button type="submit" name="action" value="save">저장</button>
        <button type="submit" name="action" value="restart">저장 & 리스타트</button>
    </form>
    <a href="javascript:history.back()">뒤로가기</a>
    `, path, path, data)
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(html))
}

// 파일 저장
func saveFile(c *gin.Context) {
	path := c.PostForm("path")
	action := c.PostForm("action")
	newContent := c.PostForm("content")
	if path == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	// 백업
	if err := backupFile(path); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("백업 실패: %v", err))
		return
	}
	// 저장
	if err := ioutil.WriteFile(path, []byte(newContent), 0644); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("파일 저장 실패: %v", err))
		return
	}
	msg := "파일 저장 완료!"
	if action == "restart" {
		out, err := dockerComposeRestart(path)
		if err != nil {
			msg += fmt.Sprintf("<br/>도커 재시작 오류: %v<br/>출력:%s", err, out)
		} else {
			msg += "<br/>도커 재시작 완료!"
		}
	}
	c.String(http.StatusOK, msg)
}

// 도커 재시작
func restartDocker(c *gin.Context) {
	path := c.PostForm("path")
	if path == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	out, err := dockerComposeRestart(path)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력:%s", err, out))
		return
	}
	c.String(http.StatusOK, "도커 재시작 완료!")
}

// 백업 목록
func listRevisions(c *gin.Context) {
	path := c.Query("path")
	if path == "" {
		c.String(http.StatusBadRequest, "path가 필요합니다.")
		return
	}
	fileName := filepath.Base(path)
	ext := filepath.Ext(fileName)
	base := fileName[0 : len(fileName)-len(ext)]

	files, err := ioutil.ReadDir(backupDir)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("백업 디렉토리 조회 오류: %v", err))
		return
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("<h1>[%s] 백업 파일 목록</h1><ul>", path))
	for _, f := range files {
		if !f.IsDir() && strings.HasPrefix(f.Name(), base+"_") {
			sb.WriteString(fmt.Sprintf(`
            <li>%s 
              <a href="/file/download?backupfile=%s" target="_blank">[다운로드]</a>
              <form style="display:inline;" method="POST" action="/file/rollback">
                <input type="hidden" name="backupfile" value="%s"/>
                <input type="hidden" name="target" value="%s"/>
                <input type="submit" value="롤백" onclick="return confirm('해당 백업으로 롤백?');"/>
              </form>
            </li>`, f.Name(), f.Name(), f.Name(), path))
		}
	}
	sb.WriteString("</ul>")
	sb.WriteString(fmt.Sprintf(`<a href="/file/edit?path=%s">편집화면</a>`, path))
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(sb.String()))
}

// 롤백
func rollbackFile(c *gin.Context) {
	backupfile := c.PostForm("backupfile")
	target := c.PostForm("target")
	if backupfile == "" || target == "" {
		c.String(http.StatusBadRequest, "backupfile, target 모두 필요합니다.")
		return
	}
	backupPath := filepath.Join(backupDir, backupfile)
	data, err := ioutil.ReadFile(backupPath)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("백업 파일 읽기 실패: %v", err))
		return
	}
	if err := ioutil.WriteFile(target, data, 0644); err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("롤백 실패: %v", err))
		return
	}
	out, err := dockerComposeRestart(target)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("도커 재시작 오류: %v\n출력:%s", err, out))
		return
	}
	c.String(http.StatusOK, "롤백 및 도커 재시작 완료!")
}

// 백업 파일 다운로드
func downloadBackupFile(c *gin.Context) {
	backupfile := c.Query("backupfile")
	if backupfile == "" {
		c.String(http.StatusBadRequest, "backupfile 파라미터가 없습니다.")
		return
	}
	backupPath := filepath.Join(backupDir, backupfile)
	f, err := os.Open(backupPath)
	if err != nil {
		c.String(http.StatusInternalServerError, fmt.Sprintf("파일 열기 실패: %v", err))
		return
	}
	defer f.Close()
	c.Header("Content-Disposition", "attachment; filename="+backupfile)
	c.Header("Content-Type", "application/octet-stream")
	io.Copy(c.Writer, f)
}

// ---------------------------------------------------
// 8. 백업 & 도커 재시작
// ---------------------------------------------------

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

// ---------------------------------------------------
// 9. 기타
// ---------------------------------------------------

func sendApprovalEmail(newUserEmail string) {
	fmt.Printf("[이메일 발송] 신규 회원(%s) 가입! 기존 어드민에게 승인 요청\n", newUserEmail)
}

