package telegrambot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/acarl005/stripansi"
	jsoniter "github.com/json-iterator/go"
	"github.com/pkg/errors"
	"github.com/ruslanBik4/logs"
	"github.com/valyala/fasthttp"
	"gopkg.in/yaml.v3"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

// TelegramKeyboard struct for telegram reply_markup keyboard
type TelegramKeyboard struct {
	Keyboard        [][]string `json:"keyboard"`
	OneTimeKeyboard bool       `json:"one_time_keyboard"`
	ResizeKeyboard  bool       `json:"resize_keyboard"`
}

// TelegramBot struct with token and one chats id
type TelegramBot struct {
	Token          string `yaml:"BotToken"`
	ChatID         string `yaml:"ChatID"`
	RequestURL     string
	Request        *fasthttp.Request
	Response       *fasthttp.Response
	FastHTTPClient *fasthttp.Client

	props map[string]interface{}

	currentMsg    int
	messagesStack []*tbMessageBuffer
	instance      string
	messId        int64
	lock          sync.RWMutex
}

// NewTelegramBot reads a config file for bot token and chatID and creates new TelegramBot struct
func NewTelegramBot(confPath string) (tb *TelegramBot, err error) {

	yamlFile, err := ioutil.ReadFile(confPath)
	if err != nil {
		return nil, err
	}

	err = yaml.Unmarshal(yamlFile, &tb)
	if err != nil {
		logs.DebugLog(" %+v", string(yamlFile))
		return nil, err
	}

	tb.RequestURL = baseURL
	tb.Request = &fasthttp.Request{}
	tb.Response = &fasthttp.Response{}
	tb.FastHTTPClient = &fasthttp.Client{}
	tb.allocStack()

	return
}

// NewTelegramBotFromEnv is a constructor from ENV
func NewTelegramBotFromEnv() (tb *TelegramBot, err error) {
	errMsg := ""
	tbToken := os.Getenv(TB_TOKEN)
	if tbToken == "" {
		errMsg += TB_TOKEN
	}

	tbChatId := os.Getenv(CHAT_ID)
	if tbChatId == "" {
		errMsg += " " + CHAT_ID
	}

	if errMsg > "" {
		return nil, errors.New("Empty environment variables: " + errMsg + "for TelegramBot creation.")
	}

	tb = &TelegramBot{
		Token:          tbToken,
		ChatID:         tbChatId,
		Response:       &fasthttp.Response{},
		RequestURL:     baseURL,
		Request:        &fasthttp.Request{},
		FastHTTPClient: &fasthttp.Client{},
		instance:       "[[#" + filepath.Base(os.Args[0]) + "]] ",
	}
	tb.Request.Header.SetMethod(fasthttp.MethodPost)
	tb.allocStack()

	err, resp := tb.SendMessage("Telegram Bot ready", false)
	if err == ErrBadTelegramBot {
		return nil, errors.Errorf(
			"%s, StatusCode: %d Description: %s",
			ErrBadTelegramBot,
			resp.ErrorCode,
			resp.Description)
	} else if err != nil {
		return nil, err
	}

	return tb, nil

}

func (tb *TelegramBot) String() string {
	msgStr := ""
	for key, msg := range tb.messagesStack {
		msgStr += fmt.Sprintf("%d: %s,", key, msg)
	}
	return fmt.Sprintf(
		"TelegramBot: {Token: %s, ChatID: %s, RequestURL: %s, Request: %s, Response: %s, instance: %s, messagesStack: %s}",
		tb.Token, tb.ChatID, tb.RequestURL, tb.Request.String(), tb.Response.String(), tb.instance, msgStr)
}

// TelegramBotHandler reads bot params from configPath and accepts some log struct to find if its needed to print some mess to telegram bot
func (tb *TelegramBot) Write(msg []byte) (int, error) {
	if tb.msgInStack(msg) {
		return len(msg), nil
	}

	err, _ := tb.SendMessage(b2s(msg), false)
	if err == ErrBadTelegramBot {
		return -1, logs.ErrBadWriter
	}

	if err != nil {
		return -1, err
	}

	tb.putMsgStack(msg)

	return len(msg), nil
}

// GetUpdates is used for getting updates for bot
func (tb *TelegramBot) GetUpdates() error {
	err, _ := tb.FastRequest(cmdgetUpdates, map[string]string{})
	if err != nil {
		return err
	}

	return tb.readResponse(err)
}

// GetChat is used for sending messages
func (tb *TelegramBot) GetChat(name string) error {
	err, _ := tb.FastRequest(cmdGetChat,
		map[string]string{
			"chat_id": name,
		})
	if err != nil {
		return err
	}

	return tb.readResponse(err)
}

// GetChatMemberCount is used counting chat members
func (tb *TelegramBot) GetChatMemberCount(name string) error {
	err, _ := tb.FastRequest(cmdGetChMbrsCount,
		map[string]string{
			"chat_id": name,
		})
	if err != nil {
		return err
	}

	return tb.readResponse(err)
}

// GetChatMember is used for getting chat member
func (tb *TelegramBot) GetChatMember(name string, user string) error {
	err, _ := tb.FastRequest(cmdGetChMbr,
		map[string]string{
			"chat_id": name,
			"user_id": user,
		})
	if err != nil {
		return err
	}

	return tb.readResponse(err)
}

//GetResult
func (tb *TelegramBot) GetResult() interface{} {
	return tb.props["result"]
}

// InviteUser is for inviting members
func (tb *TelegramBot) InviteUser(name string) error {
	err, _ := tb.FastRequest(cmdInlineMThd,
		map[string]string{
			"chat_id": name,
		})
	if err != nil {
		return err
	}

	return nil
}

// SendMessage is used for sending messages. Arguments keys must contain TelegramKeyboard{} to add keys to your message
func (tb *TelegramBot) SendMessage(message string, markdown bool, keys ...interface{}) (err error, response *TbResponseMessageStruct) {
	if tb == nil {
		return errors.New("tb is nil"), nil
	}

	if err := tb.checkBot(); err != nil {
		return err, nil
	}

	if string(tb.Request.Header.Method()) == "GET" {
		strings.Replace(message, " ", "%20", -1)
	}

	requestParams := map[string]string{
		"chat_id": tb.ChatID,
	}
	message = strings.TrimSpace(message)

	if markdown {
		requestParams["parse_mode"] = "Markdown"
	} else {
		message = stripansi.Strip(message)
	}

	if keys != nil {
		replyMarkup, checkType := keys[0].(TelegramKeyboard)

		if checkType == true {
			keysJsonString, err := json.Marshal(replyMarkup)
			if err != nil {
				logs.ErrorStack(err)
			} else {
				requestParams["reply_markup"] = string(keysJsonString)
			}
		}
	}

	messNum := atomic.LoadInt64(&tb.messId)

	switch messLen := len(message); {
	case messLen == 0:
		logs.ErrorStack(errors.Wrap(ErrEmptyMessText, message))
	case messLen+len(tb.instance) > maxMessLength:
		prefix := " part 1 "

		r := strings.NewReader(message)

		for i := 1; r.Len() > 0; i++ {

			requestParams["text"], err = tb.getPartMes(r, prefix, messNum+1)
			if err == ErrEmptyMessText {
				logs.GetStack(2, fmt.Sprintf("%v (%s) part#%d", err, message, i))
				return
			} else if err != nil {
				return
			}

			err, response = tb.FastRequest(cmdSendMes, requestParams)
			if err != nil {
				return err, response
			}

			prefix = fmt.Sprintf(" MESS #%v part %d ", tb.messId, i+1)
		}
	default:
		requestParams["text"] = tb.instance + message
		err, response = tb.FastRequest(cmdSendMes, requestParams)
	}

	return
}

// checks bot params, used in send message, can be used in other methods
func (tb *TelegramBot) checkBot() error {
	if tb.Token == "" {
		return ErrBadBotParams{"TelegramBot.Token empty"}
	}
	if tb.ChatID == "" {
		return ErrBadBotParams{"TelegramBot.ChatID empty"}
	}
	if tb.FastHTTPClient == nil {
		return ErrBadBotParams{"TelegramBot.FastHTTPClient == nil"}
	}
	if tb.Request == nil {
		return ErrBadBotParams{"TelegramBot.Request == nil"}
	}
	if tb.Response == nil {
		return ErrBadBotParams{"TelegramBot.Response == nil"}
	}

	return nil
}

func (tb *TelegramBot) getPartMes(r *strings.Reader, prefix string, num int64) (string, error) {
	suffix := fmt.Sprintf(" MESS #%v  CONTINUE->", num)
	buf := make([]byte, maxMessLength-len(tb.instance)-len(prefix)-len(suffix))

	c, err := r.Read(buf)
	if err != nil {
		return "", errors.Wrapf(err, "read message (%d bytes read)", c)
	}

	if c < len(buf) {
		buf = buf[:c]
	}

	if r.Len() <= 0 {
		suffix = fmt.Sprintf(" MESS #%v ENDED", tb.messId)
	}

	mes := tb.instance + prefix + string(buf) + suffix
	if len(mes) == 0 {
		return "", ErrEmptyMessText
	}

	return mes, nil
}

func (tb *TelegramBot) putMsgStack(msg []byte) {
	if len(tb.messagesStack) == 0 {
		tb.allocStack()
	}

	tb.lock.Lock()
	defer tb.lock.Unlock()

	tb.currentMsg++
	if tb.currentMsg == maxStack {
		tb.currentMsg = 0
	}

	tb.messagesStack[tb.currentMsg] = newtbMessageBuffer(msg)
}

func (tb *TelegramBot) msgInStack(msg []byte) bool {
	tb.lock.RLock()
	defer tb.lock.RUnlock()

	for _, v := range tb.messagesStack {
		if v != nil && bytes.Equal(v.messageText, msg) {
			return true
		}
	}

	return false
}

func (tb *TelegramBot) allocStack() {
	tb.lock.Lock()
	defer tb.lock.Unlock()

	tb.messagesStack = make([]*tbMessageBuffer, maxStack)
}

func (tb *TelegramBot) readResponse(err error) error {
	d := tb.Response.Body()

	enc := jsoniter.NewDecoder(bytes.NewReader(d))

	err = enc.Decode(&tb.props)
	if err != nil {
		return err
	}

	return nil
}
