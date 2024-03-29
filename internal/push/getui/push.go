package getui

import (
	"Open_IM/pkg/common/config"
	"Open_IM/pkg/common/db"
	"Open_IM/pkg/common/log"
	"Open_IM/pkg/utils"
	"bytes"
	"crypto/sha256"
	//"crypto/sha512"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"net/http"
	"strconv"
	"time"
)

var (
	GetuiClient *Getui
)

const (
	PushURL = "/push/single/cid"
	AuthURL = "/auth"
)

func init() {
	GetuiClient = newGetuiClient()
}

type Getui struct{}

type GetuiCommonResp struct {
	Code int         `json:"code"`
	Msg  string      `json:"msg"`
	Data interface{} `json:"data"`
}

type AuthReq struct {
	Sign      string `json:"sign"`
	Timestamp string `json:"timestamp"`
	Appkey    string `json:"appkey"`
}

type AuthResp struct {
	ExpireTime string `json:"expire_time"`
	Token      string `json:"token"`
}

type PushReq struct {
	RequestID string `json:"request_id"`
	Audience  struct {
		Cid []string `json:"cid"`
	} `json:"audience"`
	PushMessage struct {
		Notification Notification `json:"notification,omitempty"`
		Transmission string       `json:"transmission,omitempty"`
	} `json:"push_message"`
}

type Notification struct {
	Title     string `json:"title"`
	Body      string `json:"body"`
	ClickType string `json:"click_type"`
	//Intent    string `json:"intent,omitempty"`
	//Url       string `json:"url,omitempty"`
}

type PushResp struct {
	GetuiCommonResp
}

func newGetuiClient() *Getui {
	return &Getui{}
}

func (g *Getui) Push(userIDList []string, alert, detailContent, platform, operationID string) (resp string, err error) {
	token, err := db.DB.GetGetuiToken()
	log.NewDebug(operationID, utils.GetSelfFuncName(), "token：", token)
	if err != nil {
		log.NewError(operationID, utils.OperationIDGenerator(), "GetGetuiToken failed", err.Error())
	}
	if token == "" || err != nil {
		token, err = g.getTokenAndSave2Redis(operationID)
		if err != nil {
			log.NewError(operationID, utils.GetSelfFuncName(), "getTokenAndSave2Redis failed", err.Error())
			return "", utils.Wrap(err, "")
		}
	}
	pushReq := PushReq{
		RequestID: utils.OperationIDGenerator(),
		Audience: struct {
			Cid []string `json:"cid"`
		}{Cid: []string{userIDList[0]}},
	}
	pushReq.PushMessage.Notification = Notification{
		Title:     alert,
		Body:      alert,
		ClickType: "startapp",
	}
	pushResp := PushResp{}
	err = g.request(PushURL, pushReq, token, &pushResp, operationID)
	if err != nil {
		return "", utils.Wrap(err, "push failed")
	}
	log.NewDebug(operationID, utils.GetSelfFuncName(), "resp: ", pushResp)
	if pushResp.Code == 10001 {
		_, _ = g.getTokenAndSave2Redis(operationID)
	}
	respBytes, err := json.Marshal(pushResp)
	return string(respBytes), utils.Wrap(err, "")
}

func (g *Getui) Auth(operationID string, timeStamp int64) (token string, expireTime int64, err error) {
	log.NewInfo(operationID, utils.GetSelfFuncName(), config.Config.Push.Getui.AppKey, timeStamp, config.Config.Push.Getui.MasterSecret)
	h := sha256.New()
	h.Write([]byte(config.Config.Push.Getui.AppKey + strconv.Itoa(int(timeStamp)) + config.Config.Push.Getui.MasterSecret))
	sum := h.Sum(nil)
	sign := hex.EncodeToString(sum)
	log.NewInfo(operationID, utils.GetSelfFuncName(), "sha256 result", sign)
	reqAuth := AuthReq{
		Sign:      sign,
		Timestamp: strconv.Itoa(int(timeStamp)),
		Appkey:    config.Config.Push.Getui.AppKey,
	}
	respAuth := AuthResp{}
	err = g.request(AuthURL, reqAuth, "", &respAuth, operationID)
	if err != nil {
		return "", 0, err
	}
	log.NewInfo(operationID, utils.GetSelfFuncName(), "result: ", respAuth)
	expire, err := strconv.Atoi(respAuth.ExpireTime)
	return respAuth.Token, int64(expire), err
}

func (g *Getui) request(url string, content interface{}, token string, returnStruct interface{}, operationID string) error {
	con, err := json.Marshal(content)
	if err != nil {
		return err
	}
	client := &http.Client{}
	log.Debug(operationID, utils.GetSelfFuncName(), "json:", string(con))
	req, err := http.NewRequest("POST", config.Config.Push.Getui.PushUrl+url, bytes.NewBuffer(con))
	if err != nil {
		return err
	}
	if token != "" {
		req.Header.Set("token", token)
	}
	req.Header.Set("content-type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	result, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	log.NewInfo(operationID, "getui", utils.GetSelfFuncName(), "resp, ", string(result))
	commonResp := GetuiCommonResp{}
	commonResp.Data = returnStruct
	if err := json.Unmarshal(result, &commonResp); err != nil {
		return err
	}
	return nil
}

func (g *Getui) getTokenAndSave2Redis(operationID string) (token string, err error) {
	token, expireTime, err := g.Auth(operationID, time.Now().UnixNano()/1e6)
	if err != nil {
		return "", utils.Wrap(err, "Auth failed")
	}
	log.NewDebug(operationID, "getui", utils.GetSelfFuncName(), token, expireTime, err)
	err = db.DB.SetGetuiToken(token, 60*60*23)
	if err != nil {
		return "", utils.Wrap(err, "Auth failed")
	}
	return token, nil
}
