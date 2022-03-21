package telegrambot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"strings"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/ruslanBik4/logs"
	"github.com/valyala/fasthttp"
)

// FastRequest make fasthttp request
func (tb *TelegramBot) FastRequest(action string, params map[string]string) (error, *TbResponseMessageStruct) {
	tb.lock.Lock()
	defer tb.lock.Unlock()

	tb.setRequestURL(action)
	err := tb.setMultipartData(params)
	if err != nil {
		return err, nil
	}
	tryCounter := 0

	for {
		err := tb.FastHTTPClient.DoTimeout(tb.Request, tb.Response, time.Minute)
		switch err {
		case fasthttp.ErrTimeout, fasthttp.ErrDialTimeout:
			logs.DebugLog("eErrTimeout")
			<-time.After(time.Minute * 2)
			continue
		case fasthttp.ErrNoFreeConns:
			logs.DebugLog("ErrTimeout")
			<-time.After(time.Minute * 2)
			continue
		case nil:
			var resp = &TbResponseMessageStruct{}
			err = json.Unmarshal(tb.Response.Body(), resp)
			switch tb.Response.StatusCode() {
			case 400:
				if strings.Contains(resp.Description, "message text is empty") {
					logs.GetStack(3, fmt.Sprintf("%v (%s) %+v", ErrEmptyMessText, params["text"], resp))
					return nil, resp
				} else if strings.Contains(resp.Description, "message is too long") {
					logs.ErrorStack(errors.Wrap(ErrTooLongMessText, ""))
					return nil, resp
				}
				logs.DebugLog("tb response 400, ResponseStruct:", resp.ErrorCode, resp.Description)
				return ErrBadTelegramBot, resp
			case 404:
				logs.DebugLog("tb response 404, ResponseStruct:", resp.ErrorCode, resp.Description)
				return ErrBadTelegramBot, resp
			case 429:
				<-time.After(time.Second * 1)
			case 500:
				if tryCounter > 100 {
					return ErrTelegramBotMultiple500, resp
				} else {
					tryCounter += 1
					<-time.After(time.Second * 10)
				}
			default:
				if !resp.Ok {
					// todo: add parsing error response
					logs.DebugLog(resp)
				}

				if action == cmdSendMes {
					atomic.AddInt64(&tb.messId, 1)
				}

				return nil, resp
			}

		default:
			if strings.Contains(err.Error(), "connection reset by peer") {
				logs.DebugLog(err.Error())
				<-time.After(time.Minute * 2)
				continue
			} else {
				return err, nil
			}
		}
	}
}

// setRequestURL makes url for request
func (tb *TelegramBot) setRequestURL(action string) {
	newUrl := tb.RequestURL + tb.Token + "/" + action
	if string(tb.Request.Header.Method()) == "GET" {
		newUrl += "?"
	}
	tb.Request.SetRequestURI(newUrl)
}

// Set multipart data for request
func (tb *TelegramBot) setMultipartData(params map[string]string) error {
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	for name, val := range params {
		err := w.WriteField(name, val)
		if err != nil {
			return err
		}
	}

	if err := w.Close(); err != nil {
		return err
	}

	tb.Request.Header.Set("Content-Type", w.FormDataContentType())
	tb.Request.SetBody(b.Bytes())
	return nil
}
