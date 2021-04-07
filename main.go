package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
	"github.com/google/uuid"
	"gopkg.in/olahol/melody.v1"
)

type Message struct {
	Event   string `json:"event"`
	Name    string `json:"name"`
	Content string `json:"content"`
}

const (
	KEY  = "chat_id"
	WAIT = "wait"
)

var redisClient *redis.Client

func init() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	pong, err := redisClient.Ping(context.Background()).Result()
	if err == nil {
		log.Println("redis 回應成功，", pong)
	} else {
		log.Fatal("redis 無法連線，錯誤為", err)
	}
}

func main() {
	r := gin.Default()
	r.LoadHTMLGlob("template/html/*")
	r.Static("/assets", "./template/assets")
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	// 建立 melody 物件
	m := melody.New()

	// gin => routing, melody => request
	r.GET("/ws", func(c *gin.Context) {
		m.HandleRequest(c.Writer, c.Request)
	})

	// 連線處理
	// m.HandleConnect(func(session *melody.Session) {
	// 	id := session.Request.URL.Query().Get("id")
	// 	m.Broadcast(NewMessage("other", id, "加入聊天室").GetByteMessage())
	// })
	m.HandleConnect(func(session *melody.Session) {
		id := InitSession(session)
		if key, err := GetWaitFirstKey(); err == nil && key != "" {
			CreateChat(id, key)
			msg := NewMessage("other", "對方已經", "加入聊天室").GetByteMessage()
			m.BroadcastFilter(msg, func(session *melody.Session) bool {
				compareID, _ := session.Get(KEY)
				return compareID == id || compareID == key
			})
		} else {
			AddToWaitList(id)
		}
	})

	// 離線處理
	m.HandleClose(func(session *melody.Session, i int, s string) error {
		// id := session.Request.URL.Query().Get("id")
		// m.Broadcast(NewMessage("other", id, "離開聊天室").GetByteMessage())
		// return nil
		id := GetSessionID(session)
		chatTo, _ := redisClient.Get(context.TODO(), id).Result()
		msg := NewMessage("other", "對方已經", "離開聊天室").GetByteMessage()
		RemoveChat(id, chatTo)
		return m.BroadcastFilter(msg, func(session *melody.Session) bool {
			compareID, _ := session.Get(KEY)
			return compareID == chatTo
		})
	})

	// 訊息處理
	m.HandleMessage(func(s *melody.Session, msg []byte) {
		// m.Broadcast(msg)
		id := GetSessionID(s)
		chatTo, _ := redisClient.Get(context.TODO(), id).Result()
		m.BroadcastFilter(msg, func(session *melody.Session) bool {
			compareID, _ := session.Get(KEY)
			return compareID == chatTo || compareID == id
		})
	})

	r.Run(":5000")
}

func InitSession(s *melody.Session) string {
	id := uuid.New().String()
	s.Set(KEY, id)
	return id
}

func GetSessionID(s *melody.Session) string {
	if id, isExist := s.Get(KEY); isExist {
		return id.(string)
	}
	return InitSession(s)
}

func GetWaitFirstKey() (string, error) {
	return redisClient.LPop(context.Background(), WAIT).Result()
}

func AddToWaitList(id string) error {
	return redisClient.LPush(context.Background(), WAIT, id).Err()
}

func CreateChat(id1, id2 string) {
	redisClient.Set(context.Background(), id1, id2, 0)
	redisClient.Set(context.Background(), id2, id1, 0)
}

func RemoveChat(id1, id2 string) {
	redisClient.Del(context.Background(), id1, id2)
}

func NewMessage(event, name, content string) *Message {
	return &Message{
		Event:   event,
		Name:    name,
		Content: content,
	}
}

func (m *Message) GetByteMessage() []byte {
	result, _ := json.Marshal(m)
	return result
}
