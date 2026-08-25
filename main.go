/*
taswira: tiny image host authed by forgejo

env vars:

	TASWIRA_INSTANCE="https://example.com" # Forgejo instance to use for auth, without trailing slash
	TASWIRA_IMG_ROOT="/path/to/image/dir" # dir to write images to, defaults to <process cwd>/img
	TASWIRA_SUBPATH="foo/bar/baz" # reverse proxy subpath, without trailing slash
	TASWIRA_LISTEN_PORT="6969" # listening port, default 6969; 0 disables
	TASWIRA_UNIX_SOCKET="./taswira.sock" # listening socket, default <cwd>/taswira.sock
*/
package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	"image/png"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	taswira "git.0xf0xx0.eth.limo/0xf0xx0/taswira/auth_wrappers"
	"git.0xf0xx0.eth.limo/0xf0xx0/taswira/common"
	_ "github.com/jdeng/goheif"
	"github.com/zeebo/xxh3"
	_ "golang.org/x/image/webp"
)

type response struct {
	Message string `json:"message"`
	Url     string `json:"url,omitempty"`
}
type errorResponse struct {
	Message string `json:"error"`
	Url     string `json:"url,omitempty"`
}

const MAX_BODY = 1024 * 1024 * 256

var (
	SUBPATH  = os.Getenv("TASWIRA_SUBPATH")
	IMG_ROOT = func() string {
		env := os.Getenv("TASWIRA_IMG_ROOT")
		if env == "" {
			env = "./img"
		}
		return env
	}()
	LISTEN_PORT = func() int {
		env := os.Getenv("TASWIRA_LISTEN_PORT")
		if env == "" {
			return 6969
		}
		i, err := strconv.Atoi(env)
		if err != nil {
			log.Fatalln(err)
		}
		/// clamp to port range
		/// yes this overflows, no we dont care
		/// config your shit right lmao
		return int(uint16(i))
	}()
	UNIX_SOCKET = func() string {
		env := os.Getenv("TASWIRA_UNIX_SOCKET")
		if env == "" {
			env = "./taswira.sock"
			// env = "/var/run/taswira.sock"
		}
		return env
	}()
	imgroot *os.Root
)

func main() {
	var err error
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	if SUBPATH != "" {
		SUBPATH += "/"
	}

	imgroot, err = os.OpenRoot(IMG_ROOT)
	if err != nil {
		log.Fatalln(err)
	}

	http.HandleFunc("/", mainHandler)

	go func() {
		if LISTEN_PORT == 0 {
			log.Println("not listening on http")
			return
		}
		log.Printf("listening on http://localhost:%d\n", LISTEN_PORT)
		log.Fatal(http.ListenAndServe(fmt.Sprintf("localhost:%d", LISTEN_PORT), nil))
	}()
	go func() {
		if UNIX_SOCKET == "" {
			log.Println("not listening on unix socket")
			return
		}
		log.Printf("listening on unix://%s", UNIX_SOCKET)
		addr, err := net.ResolveUnixAddr("unix", UNIX_SOCKET)
		if err != nil {
			log.Fatalf("failed to resolve unix: %s", err)
		}
		l, err := net.ListenUnix("unix", addr)
		if err != nil {
			log.Fatalf("failed to listen on unix: %s", err)
		}
		http.Serve(l, nil)
	}()

	// wait for exit
	<-sigs

	if UNIX_SOCKET != "" {
		os.Remove(UNIX_SOCKET)
	}
}

// handles auth and delegates to method handlers
func mainHandler(w http.ResponseWriter, r *http.Request) {
	w.Header()["Date"] = nil
	w.Header().Set("Content-Type", "application/json")

	scheme := r.Header.Get("X-Forwarded-Proto")
	host := r.Header.Get("X-Forwarded-Host")
	if scheme == "" {
		log.Print("X-Forwarded-Proto header not set")
		return
	}
	if host == "" {
		host = r.Header.Get("Host")
	}
	if host == "" {
		log.Print("X-Forwarded-Host or Host header not set")
		return
	}

	path := fmt.Sprintf("%s://%s/%s", scheme, host, SUBPATH)
	var handler func(urlPfx, username string, r *http.Request, w http.ResponseWriter) (ok bool)

	switch r.Method {
	case "POST":
		handler = postHandler
	case "DELETE":
		handler = deleteHandler
	default:
		return
	}

	username, token, ok := r.BasicAuth()
	if !ok {
		return
	}

	var auther taswira.AuthWrapper
	/// TODO: best way to differentiate auth sources?
	switch () {
		case min
	}

	if !auther.IsAlive() {
		e := &errorResponse{
			Message: "auth backend is down",
		}
		writeError(e, w, http.StatusBadGateway)
		return
	}

	if !auther.Authenticate(username, token, w) {
		return
	}

	w.Header().Add("Server", common.USER_AGENT)

	if !handler(path, username, r, w) {
		return
	}
}

func postHandler(path, username string, r *http.Request, w http.ResponseWriter) bool {
	if r.ContentLength > MAX_BODY || r.ContentLength < 8 {
		log.Printf("ignoring image from %s: invalid Content-Length\n", username)
		e := &errorResponse{
			Message: "image too large (>256MiB)",
		}
		writeError(e, w, http.StatusRequestEntityTooLarge)
		return false
	}

	uploadBody, err := io.ReadAll(io.LimitReader(r.Body, r.ContentLength))
	if err != nil {
		log.Printf("error reading image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error reading image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}
	w.WriteHeader(http.StatusProcessing)
	decodedImg, _, err := image.Decode(bytes.NewReader(uploadBody))
	if err != nil {
		log.Printf("%q\n", uploadBody[:80])
		log.Printf("error decoding image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error decoding image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}

	/// check for dupe before saving
	encodedImg := bytes.NewBuffer(make([]byte, 0, len(uploadBody)))
	err = png.Encode(encodedImg, decodedImg)
	if err != nil {
		log.Printf("error encoding image from %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error encoding image: %s", err),
		}
		writeError(e, w, http.StatusUnprocessableEntity)
		return false
	}

	/// verify the hash after metadata removal
	hash := xxh3.Hash128(encodedImg.Bytes()).Bytes()
	filename := hex.EncodeToString(hash[:]) + ".png"
	url := path + filename
	if checkIfImageExists(filename) {
		e := &errorResponse{
			Message: "duplicate image",
			Url:     url,
		}
		writeError(e, w, http.StatusConflict)
		return false
	}

	/// write
	out, err := imgroot.Create(filename)
	if err != nil {
		log.Printf("error creating image file for %s: %s\n", username, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error creating %s", filename),
		}
		writeError(e, w, http.StatusInternalServerError)
		return false
	}
	_, err = encodedImg.WriteTo(out)
	if err != nil {
		e := &errorResponse{
			Message: fmt.Sprintf("error writing %s", filename),
		}
		log.Printf("error writing image file for %s: %s\n", username, err)
		writeError(e, w, http.StatusInternalServerError)
		return false
	}

	log.Printf("successful upload from %s: %s", username, filename)
	m := &response{
		Message: "ok",
		Url:     url,
	}
	/// 200 OK cause 201 has
	/// "The newly-created items can be returned in the body of the response message,
	///  but must be locatable by the URL of the initiating request or by the URL in
	///  the value of the Location header provided with the response."
	///	 - https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Status/201
	/// and we dont do that
	writeResponse(m, w, http.StatusOK)
	return true
}
func deleteHandler(_, username string, r *http.Request, w http.ResponseWriter) bool {
	deletehash := r.URL.Query().Get("hash")
	if deletehash == "" {
		e := &errorResponse{
			Message: "no image hash given",
		}
		writeError(e, w, http.StatusBadRequest)
		return false
	}

	fileToDelete := deletehash + ".png"
	if !checkIfImageExists(fileToDelete) {
		e := &errorResponse{
			Message: fmt.Sprintf("nonexistent image: %s", fileToDelete),
		}
		writeError(e, w, http.StatusNotFound)
		return false
	}

	if err := imgroot.Remove(fileToDelete); err != nil {
		log.Printf("error deleting %s: %s", fileToDelete, err)
		e := &errorResponse{
			Message: fmt.Sprintf("error deleting %s", fileToDelete),
		}
		writeError(e, w, http.StatusInternalServerError)
		return false
	}

	log.Printf("successful deletion from %s: %s", username, fileToDelete)
	m := &response{
		Message: "ok",
	}
	writeResponse(m, w, http.StatusNoContent)

	return true
}

var imageExistsCache = make(map[string]struct{}, 15)

func checkIfImageExists(path string) bool {
	_, ok := imageExistsCache[path]
	if ok {
		return ok
	}

	_, err := imgroot.Stat(path)
	if err == nil {
		imageExistsCache[path] = struct{}{}
		return true
	}
	return false
}

func writeResponse(res *response, w http.ResponseWriter, code int) {
	b, _ := json.Marshal(res)
	w.WriteHeader(code)
	w.Write(b)
}

func writeError(err *errorResponse, w http.ResponseWriter, statusCode int) {
	b, _ := json.Marshal(err)
	w.WriteHeader(statusCode)
	w.Write(b)
}
