package ws

import(
    "log"
    "time"
    "encoding/json"
    "net/http"
    "github.com/gorilla/websocket"
    "github.com/neophenix/lxdepot/internal/config"
)

type IncomingMessage struct {
    Action string
    Data map[string]string
}

type OutgoingMessage struct {
    Id int64
    Message string
    Success bool
    Redirect string
}

var upgrader = websocket.Upgrader{
    CheckOrigin: func(r *http.Request) bool { return true },
}

var Conf *config.Config

func Handler(w http.ResponseWriter, r *http.Request) {
    conn, err := upgrader.Upgrade(w, r, nil)
    if err != nil {
        log.Print("upgrade:", err)
        return
    }
    defer conn.Close()
    for {
        mt, enc_msg, err := conn.ReadMessage()
        if err != nil {
            log.Println("read:", err)
            break
        }
        log.Printf("ws recv: %s\n", enc_msg)
        var msg IncomingMessage
        err = json.Unmarshal(enc_msg, &msg)
        if err != nil {
            log.Println("unmarshal:", err)
            break
        }

        switch msg.Action {
            case "start":
                StartContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "stop":
                StopContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "create":
                CreateContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/container/" + msg.Data["host"] + ":" + msg.Data["name"]})
                conn.WriteMessage(mt, data)
            case "delete":
                DeleteContainerHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{Redirect: "/containers"})
                conn.WriteMessage(mt, data)
            case "bootstrap":
                ContainerBootstrapHandler(conn, mt, msg)
                data, _ := json.Marshal(OutgoingMessage{})
                conn.WriteMessage(mt, data)
            default:
                id := time.Now().UnixNano()
                data, _ := json.Marshal(OutgoingMessage{Id: id, Message:"Request not understood", Success: false})
                conn.WriteMessage(mt, data)
        }
    }
}
