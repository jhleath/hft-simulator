package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sort"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
}

var nilChan chan struct{}

func main() {
	go startTradeServer()

	go func() {
		time.Sleep(3 * time.Second)
		fmt.Println("Launching the Big Guns!")
		// newHFTTrader(newSimpleTrader(100))
		newHFTTrader(newMarketMakerTrader(100, 3*time.Second))
		<-nilChan
	}()

	http.Handle("/", http.FileServer(http.Dir("public")))

	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		// We now have a conn...
		t := newTraderConnection(conn)
		t.startConnection()
	})

	log.Fatal(http.ListenAndServe(":8080", nil))
}

type trader interface {
	writeMessage(typ string, data interface{})
}

type order struct {
	ID       float64   `json:"id"`
	Quantity int       `json:"quantity"`
	Price    float64   `json:"price"`
	Date     time.Time `json:"date"`
	Sell     bool      `json:"sell"`
	Owner    trader    `json:"-"`
}

type orderBook []*order
type sellOrderBook orderBook

func (l sellOrderBook) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l sellOrderBook) Len() int      { return len(l) }

// sells increase with price
func (l sellOrderBook) Less(i, j int) bool {
	return l[i].Price < l[j].Price
}

type buyOrderBook orderBook

func (l buyOrderBook) Swap(i, j int) { l[i], l[j] = l[j], l[i] }
func (l buyOrderBook) Len() int      { return len(l) }

// buys decrease with price
func (l buyOrderBook) Less(i, j int) bool {
	return l[i].Price > l[j].Price
}

type traderList []trader

func (t traderList) Broadcast(typ string, data interface{}) {
	for _, v := range t {
		v.writeMessage(typ, data)
	}
}

type tradeServer struct {
	SellBook sellOrderBook
	BuyBook  buyOrderBook

	OpenConnections traderList
}

type connectionInfo struct {
	trader

	Open bool
}

type cancelRequest struct {
	id float64
	t  trader
}

var (
	listChannel       chan *order          = make(chan *order)
	connectionChannel chan *connectionInfo = make(chan *connectionInfo)
	cancelChannel     chan cancelRequest   = make(chan cancelRequest)
	currentOrderId                         = 0
)

func startTradeServer() {
	server := &tradeServer{}

	for {
		select {
		case c := <-connectionChannel:
			if c.Open {
				server.OpenConnections = append(server.OpenConnections, c.trader)
			} else {
				for i, v := range server.OpenConnections {
					if v == c.trader {
						server.OpenConnections = append(
							server.OpenConnections[:i],
							server.OpenConnections[i+1:]...,
						)
						break
					}
				}
			}
		case o := <-listChannel:
			server.OpenConnections.Broadcast("newOrder", o)

			if o.Sell {
				server.SellBook = append(server.SellBook, o)
			} else {
				server.BuyBook = append(server.BuyBook, o)
			}

			// Go through order books and make any trades necessary
			server.fillOrders()
		case req := <-cancelChannel:
			found := false
			for i, v := range server.SellBook {
				if v.ID == req.id {
					v.Owner.writeMessage("cancelledOrder", map[string]interface{}{
						"id":     req.id,
						"cancel": true,
					})
					server.OpenConnections.Broadcast("cancelledOrder", map[string]interface{}{
						"id": req.id,
					})
					server.SellBook = append(
						server.SellBook[:i],
						server.SellBook[i+1:]...,
					)
					found = true
					break
				}
			}
			for i, v := range server.BuyBook {
				if v.ID == req.id {
					v.Owner.writeMessage("cancelledOrder", map[string]interface{}{
						"id":     req.id,
						"cancel": true,
					})
					server.OpenConnections.Broadcast("cancelledOrder", map[string]interface{}{
						"id": req.id,
					})
					server.BuyBook = append(
						server.BuyBook[:i],
						server.BuyBook[i+1:]...,
					)
					found = true
					break
				}
			}
			if !found {
				req.t.writeMessage("cancelledOrder", map[string]interface{}{
					"id":     req.id,
					"cancel": false,
				})
			}
		}
	}
}

func (t *tradeServer) broadcastFilledOrder(sell, buy *order, price float64, quantity int) {
	t.OpenConnections.Broadcast("filledOrder", map[string]interface{}{
		"sellOrder": sell,
		"buyOrder":  buy,
		"quantity":  quantity,
		"price":     price,
	})
}

func (t *tradeServer) deleteOrder(o *order, index int) {
	if index != -1 {
		if o.Sell {
			t.SellBook = append(t.SellBook[:index], t.SellBook[index+1:]...)
		} else {
			t.BuyBook = append(t.BuyBook[:index], t.BuyBook[index+1:]...)
		}
	}
}

func (t *tradeServer) fillOrders() {
	// Sort the order books
	sort.Stable(t.SellBook)
	sort.Stable(t.BuyBook)

	// Start the filling algorithm
	sellIndex, buyIndex := 0, 0

	fmt.Println("Attempting to fill orders...")
	for {
		if sellIndex >= len(t.SellBook) || buyIndex >= len(t.BuyBook) {
			fmt.Println("Ran out of transactions to look at.")
			return
		}

		sellOrder := t.SellBook[sellIndex]
		buyOrder := t.BuyBook[buyIndex]

		// No order can be filled...
		if buyOrder.Price < sellOrder.Price {
			return
		}
		strikePrice := math.Floor(
			(((buyOrder.Price+sellOrder.Price)/2)*100)+0.5,
		) / 100

		if buyOrder.Quantity == sellOrder.Quantity {
			// Drop them both
			t.broadcastFilledOrder(sellOrder, buyOrder, strikePrice, buyOrder.Quantity)
			t.deleteOrder(buyOrder, buyIndex)
			t.deleteOrder(sellOrder, sellIndex)
			continue
		} else if buyOrder.Quantity > sellOrder.Quantity {
			// drop sell, alter buy
			t.broadcastFilledOrder(sellOrder, buyOrder, strikePrice, sellOrder.Quantity)
			t.deleteOrder(sellOrder, sellIndex)
			buyOrder.Quantity -= sellOrder.Quantity
			continue
		} else {
			// drop buy, alter sell
			t.broadcastFilledOrder(sellOrder, buyOrder, strikePrice, buyOrder.Quantity)
			t.deleteOrder(sellOrder, sellIndex)
			buyOrder.Quantity -= sellOrder.Quantity
			continue
		}
	}
}

type traderConnection struct {
	conn        *websocket.Conn
	messageChan chan interface{}
	dataChan    chan interface{}
	quitChan    chan struct{}
}

func (r *traderConnection) writeMessage(typ string, data interface{}) {
	r.dataChan <- map[string]interface{}{
		"type":    typ,
		"payload": data,
	}
}

type traderMessage struct {
	Order    *order
	GetBooks bool `json:"getbooks"`
	CancelId float64
}

func newTraderConnection(conn *websocket.Conn) *traderConnection {
	t := &traderConnection{
		conn:        conn,
		messageChan: make(chan interface{}),
		dataChan:    make(chan interface{}),
		quitChan:    make(chan struct{}),
	}

	// tell the trading server that we are here
	connectionChannel <- &connectionInfo{t, true}
	return t
}

func (r *traderConnection) startConnection() {
	go func(mes <-chan interface{}, data <-chan interface{}, quit <-chan struct{}) {
		defer func() {
			connectionChannel <- &connectionInfo{r, false}
		}()

		for {
			var err error
			select {
			case m := <-data:
				err = r.conn.WriteJSON(m)
			case m := <-mes:
				err = r.conn.WriteJSON(map[string]interface{}{
					"type": "message",
					"data": m,
				})
			case <-quit:
				return
			}

			// Check Write Error
			if err == syscall.EPIPE {
				// Stop!
				return
			} else if err != nil {
				fmt.Println("Error writing to WS", err)
			}
		}
	}(r.messageChan, r.dataChan, r.quitChan)

	go func(conn *websocket.Conn, dataChan chan interface{}) {
		defer func() {
			connectionChannel <- &connectionInfo{r, false}
		}()

		for {
			fmt.Println("Waiting for new message.")
			_, p, err := conn.ReadMessage()
			if err != nil {
				if err == io.EOF {
					fmt.Println("Lost connection from websocket.")
					return
				} else if err.Error() == "unexpected EOF" || err.Error() == "use of closed network connection" {
					fmt.Println("Lost connection from websocket.")
					return
				}

				fmt.Println("Error reading from websocket.", err)
				continue
			}

			tempOrder := traderMessage{}
			err = json.Unmarshal(p, &tempOrder)
			if err != nil {
				fmt.Println("Error unmarshalling from websocket", err)
				continue
			}

			if tempOrder.Order != nil {
				tempOrder.Order.Owner = r
				listChannel <- tempOrder.Order
			} else if tempOrder.GetBooks {

			} else if tempOrder.CancelId != 0 {
				cancelChannel <- cancelRequest{tempOrder.CancelId, r}
			}
		}
	}(r.conn, r.dataChan)
}
