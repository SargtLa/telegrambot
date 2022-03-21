package telegrambot

import (
	"fmt"
	"time"
)

// TbResponseMessageStruct json struct to parse response
type TbResponseMessageStruct struct {
	Ok          bool   `json:"ok"`
	ErrorCode   int    `json:"error_code"`
	Description string `json:"description"`
	Result      struct {
		MessageId int `json:"message_id"`
		Chat      struct {
			Id       int64  `json:"id"`
			Title    string `json:"title"`
			Username string `json:"username"`
			Type     string `json:"type"`
		} `json:"chat"`
		Date int64  `json:"date"`
		Text string `json:"text"`
	} `json:"result"`
}

func (tbResp *TbResponseMessageStruct) String() string {
	if tbResp.Ok == true {
		return fmt.Sprintf(`message request is Ok, 
ResponseStruct: {Desc: %s, 
Result: {MessageId: %v, 
Chat:{Id: %v, Title: %s, Username: %v, Type: %v}, Date: %.19s, Text: %v }}`,
			tbResp.Description, tbResp.Result.MessageId, tbResp.Result.Chat.Id, tbResp.Result.Chat.Title,
			tbResp.Result.Chat.Username, tbResp.Result.Chat.Type,
			time.Unix(tbResp.Result.Date, 0), tbResp.Result.Text)
	}

	return fmt.Sprintf("message request is not Ok, ErrorCode:%v, %s",
		tbResp.ErrorCode,
		tbResp.Description,
	)
}

// tbMessageBuffer stack structure
type tbMessageBuffer struct {
	messageText []byte
	messageTime time.Time
}

func newtbMessageBuffer(msg []byte) *tbMessageBuffer {
	return &tbMessageBuffer{
		messageText: msg,
		messageTime: time.Now().Round(1 * time.Second),
	}
}

func (messbuf *tbMessageBuffer) String() string {
	return fmt.Sprintf("{messageText: %s, messageTime: %v}", string(messbuf.messageText), messbuf.messageTime)
}
