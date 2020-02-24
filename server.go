package main

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/moniang/chat/service"
	"github.com/moniang/chat/sql"
	"html/template"
	"net/http"
	"strconv"
	"time"
)

// WebSocket处理事件
func wsHandler(w http.ResponseWriter, r *http.Request) {

	// 判断是否为WebSocket协议
	if !websocket.IsWebSocketUpgrade(r) {
		http.Error(w, "不是webSocket协议", http.StatusBadRequest)
		return
	}
	token, ok := r.URL.Query()["token"]
	if !ok {
		token = []string{"123"}
	}
	client := service.NewSocketClient(token[0], w, r)
	// 判断Token，并获取个人信息
	user, result := sql.CheckToken(token[0])
	if !result {
		client.Conn.Close()
		return
	}

	sendOnline := false

	// 判断此用户是否已登录
	v, ok := service.SocketList.Load(user.ID)
	if ok { // 已登录，通知下线信息
		v.(service.Client).Conn.WriteJSON(&service.Message{
			ID: -1,
		})
		v.(service.Client).Conn.WriteMessage(websocket.CloseMessage, []byte{})
		v.(service.Client).Conn.Close()
		if time.Now().Unix()-v.(service.Client).UpTime > 600 {
			sendOnline = true
		} else {
			sendOnline = false
		}
	} else {
		sendOnline = true
	}

	set := &sql.Set{} // 获取用户设置
	setErr := sql.DB.Model(user).Related(&set).Error
	if setErr != nil {
		client.FontColor = "#000000"
	} else {
		client.FontColor = set.FontColor
	}
	client.Id = user.ID
	client.Name = user.Nick

	service.SocketList.Store(user.ID, *client) // 将连接信息加入列表中
	fmt.Printf("有新用户加入 Nick：%v,User:%v\n", client.Name, user.User)
	if sendOnline {
		SendAddMessage(user.Nick, user.Vip)
	} else {
		SendAddMessage(user.Nick, 0)
	}

	defer client.Conn.Close()
	for {
		var m service.Message

		messageType, byteMsg, connErr := client.Conn.ReadMessage()
		if messageType == -1 {
			return
		}

		if string(byteMsg) == "ping" { // 心跳包，直接忽略
			client.Conn.WriteMessage(websocket.PongMessage, []byte("pong"))
			continue
		}
		err := json.Unmarshal(byteMsg, &m)
		// err := client.Conn.ReadJSON(&m)

		if websocket.IsCloseError(connErr, websocket.CloseNoStatusReceived, websocket.CloseAbnormalClosure, websocket.CloseNormalClosure, websocket.CloseGoingAway) {
			service.SocketList.Delete(client.Id)
			fmt.Println("用户主动断开了连接")
			return
		}
		if err != nil {
			fmt.Println("读取数据错误", err, string(byteMsg))
			continue
		}
		v, ok = service.SocketList.Load(user.ID)
		if ok {
			m.Nick = v.(service.Client).Name
			m.FontColor = v.(service.Client).FontColor
		} else {
			m.Nick = client.Name
			m.FontColor = client.FontColor
		}
		m.ID = client.Id

		m.SendTime = time.Now().Unix()
		m.Message = template.HTMLEscapeString(m.Message)
		// TODO:判断消息长度
		message, err := json.Marshal(m)
		if err != nil {
			fmt.Println("转换数据错误", err)
			continue
		}
		SendMessage("Message", message)
		fmt.Printf("获取到数据: %#v\n", m)
	}
}

// 发送上线消息
func SendAddMessage(nick string, vip int) {
	addMsg, err := json.Marshal(&service.Message{
		ID:      -2,
		Nick:    nick,
		Message: strconv.Itoa(vip),
	})

	if err == nil {
		SendMessage("Message", addMsg)
	}
}
