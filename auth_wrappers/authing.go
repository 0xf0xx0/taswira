package taswira

import (
	"net/http"

	"git.0xf0xx0.eth.limo/0xf0xx0/taswira/common"
)

var (
	authHttpClient = &http.Client{}
)

type AuthWrapper interface {
	IsAlive() bool
	Authenticate(username, token string, w http.ResponseWriter) (ok bool)
}

func setUA(req *http.Request) {
	req.Header.Add("User-Agent", common.USER_AGENT)
}
