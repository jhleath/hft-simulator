package main

import (
	"fmt"
	"math/rand"
	"sort"
	"time"
)

type traderBook struct {
	SellBook sellOrderBook
	BuyBook  buyOrderBook
}

func (t *traderBook) filledOrder(o *order, q int) {
	if o.Sell {
		for i, v := range t.SellBook {
			if v.ID == o.ID {
				v.Quantity -= q
				if v.Quantity == 0 {
					t.SellBook = append(t.SellBook[:i], t.SellBook[i+1:]...)
				}
				return
			}
		}
	} else {
		for i, v := range t.BuyBook {
			if v.ID == o.ID {
				v.Quantity -= q
				if v.Quantity == 0 {
					t.BuyBook = append(t.BuyBook[:i], t.BuyBook[i+1:]...)
				}
				return
			}
		}
	}
}

func (t *traderBook) newOrder(o *order) {
	copiedOrder := &order{}
	*copiedOrder = *o

	if o.Sell {
		t.SellBook = append(t.SellBook, copiedOrder)
		sort.Stable(t.SellBook)
	} else {
		t.BuyBook = append(t.BuyBook, copiedOrder)
		sort.Stable(t.BuyBook)
	}
}

func (t *traderBook) Bid() float64 {
	if len(t.BuyBook) == 0 {
		return -1
	}

	return t.BuyBook[0].Price
}

func (t *traderBook) Ask() float64 {
	if len(t.SellBook) == 0 {
		return -1
	}

	return t.SellBook[0].Price
}

type hftTrader struct {
	autoTrader
}

func newHFTTrader(a autoTrader) *hftTrader {
	t := &hftTrader{a}

	connectionChannel <- &connectionInfo{
		trader: t,
		Open:   true,
	}

	return t
}

func (s *hftTrader) writeMessage(typ string, data interface{}) {
	if typ == "newOrder" {
		out := s.newOrder(data.(*order))
		s.placeOrder(out...)
	} else if typ == "cancelledOrder" {

	} else if typ == "filledOrder" {
		m := data.(map[string]interface{})
		out := s.filledOrder(
			m["sellOrder"].(*order),
			m["buyOrder"].(*order),
			m["quantity"].(int),
			m["price"].(float64),
		)
		s.placeOrder(out...)
	} else if typ == "stopRound" {
		s.shutdown()
		connectionChannel <- &connectionInfo{s, false}
	} else {
		fmt.Println("Didn't respond to type...", typ)
	}
}

func (s *hftTrader) placeOrder(o ...*order) {
	go func() {
		for _, v := range o {
			v.Owner = s
			listChannel <- v
		}
	}()
}

type autoTrader interface {
	newOrder(o *order) []*order
	filledOrder(sell *order, buy *order, q int, p float64) []*order
	shutdown()
}

type simpleTrader struct {
	Cash  float64
	Stock int
	Mean  float64

	books       *traderBook
	Outstanding map[float64]*order
	MyTrades    map[float64]*order
}

func newSimpleTrader(mean float64) *simpleTrader {
	price := (rand.Float64() * 80) + 60

	return &simpleTrader{
		Mean:        mean,
		Cash:        2000 - (price * 10),
		Stock:       10,
		books:       &traderBook{},
		Outstanding: make(map[float64]*order),
		MyTrades:    make(map[float64]*order),
	}
}

func (s *simpleTrader) makeOrder(q int, p float64, sell bool) *order {
	return &order{
		ID:       rand.Float64(),
		Quantity: q,
		Price:    p,
		Sell:     sell,
		Date:     time.Now(),
	}
}

func (s *simpleTrader) filledOrder(sell *order, buy *order, q int, p float64) (out []*order) {
	s.books.filledOrder(sell, q)
	s.books.filledOrder(buy, q)

	// fmt.Println("Filled order sell", sell.ID, "buy", buy.ID)
	if _, sok := s.Outstanding[sell.ID]; sok {
		s.Cash += float64(q) * p
		delete(s.Outstanding, sell.ID)

		bid := s.books.Bid() + 1
		if bid == 0 {
			bid = 99.99
		}

		if bid < s.Mean {
			fmt.Println("REBUYING STOCK AT", bid)
			out = append(out, s.makeOrder(q, bid, false))
			s.Cash -= float64(q) * bid
		} else {
			fmt.Println("Won't rebuy stock...", bid)
		}
	} else if _, bok := s.Outstanding[buy.ID]; bok {
		s.Stock += q
		delete(s.Outstanding, buy.ID)

		ask := s.books.Ask() - 1
		if ask == -2 {
			ask = 100.01
		}

		if ask > s.Mean {
			fmt.Println("RESELLING STOCK AT", ask)
			out = append(out, s.makeOrder(q, ask, true))
			s.Stock -= q
		} else {
			fmt.Println("Won't resell stock...", ask)
		}

	}

	fmt.Println("HFT Trader Update", s.Cash, "dollars", s.Stock, "shares")
	return
}

func (s *simpleTrader) newOrder(o *order) (out []*order) {
	s.books.newOrder(o)

	// Don't trade on our trades
	if _, ok := s.MyTrades[o.ID]; ok {
		return
	}

	if (o.Sell && o.Price < s.Mean) || (!o.Sell && o.Price > s.Mean) {
		fmt.Println("Taking advantage of Arbitrage condition.", !o.Sell, o.Quantity, o.Price)

		newOrder := &order{
			ID:       rand.Float64(),
			Quantity: o.Quantity,
			Price:    o.Price,
			Date:     time.Now(),
			Sell:     !o.Sell,
		}
		s.Outstanding[newOrder.ID] = newOrder
		out = append(out, newOrder)
		if newOrder.Sell {
			s.Stock -= o.Quantity
		} else {
			s.Cash -= float64(o.Quantity) * o.Price
		}

		s.Outstanding[newOrder.ID] = newOrder
	}

	return
}

func (s *simpleTrader) shutdown() {}

type marketMakerTrader struct {
	*simpleTrader

	Timer   *time.Timer
	Quit    chan struct{}
	Timeout time.Duration
}

func newMarketMakerTrader(mean float64, timeout time.Duration) *marketMakerTrader {
	simpleTrader := newSimpleTrader(mean)
	timer := time.NewTimer(timeout)
	quit := make(chan struct{})

	go func() {
		for {
			select {
			case <-timer.C:
				// fmt.Println("Let's make a new trade.")
				// Now, we make the market immediately
				bid := simpleTrader.books.Bid()
				ask := simpleTrader.books.Ask()

				if bid != -1 || ask != -1 {
					// we have trades
					if bid == -1 && ask > 101 {
						fmt.Println("Making it at", ask-1)
						sellOrder, buyOrder := simpleTrader.makeOrder(1, ask-1, true), simpleTrader.makeOrder(1, ask-1, false)
						simpleTrader.MyTrades[sellOrder.ID] = sellOrder
						simpleTrader.MyTrades[buyOrder.ID] = buyOrder
						listChannel <- sellOrder
						listChannel <- buyOrder
					} else if (bid+1 < ask) || (ask == -1 && bid < 99) {
						fmt.Println("Making it at", bid+1)
						sellOrder, buyOrder := simpleTrader.makeOrder(1, bid+1, true), simpleTrader.makeOrder(1, bid+1, false)
						simpleTrader.MyTrades[sellOrder.ID] = sellOrder
						simpleTrader.MyTrades[buyOrder.ID] = buyOrder
						listChannel <- sellOrder
						listChannel <- buyOrder
					}
				}

				timer.Reset(timeout)
			case <-quit:
				return
			}
		}
	}()

	return &marketMakerTrader{
		simpleTrader: simpleTrader,
		Timer:        timer,
		Timeout:      timeout,
		Quit:         quit,
	}
}

func (m *marketMakerTrader) shutdown() {
	m.Quit <- struct{}{}
}

func (m *marketMakerTrader) newOrder(o *order) []*order {
	m.Timer.Reset(m.Timeout)

	return m.simpleTrader.newOrder(o)
}

// listChannel *order
// connectionChannel *connectionInfo

// 2 strategies
// 1.
// Knows the distribution of values, 60 - 140, mean of 100
// Whenever he sees stock for sale, selling below 100 (more likely to be taken by other people)
// take this order first
// Resell the stock at 100
// Whenever buying order is above 100, he will sell to that order
// puts a buy order in at 100
// catch bid/ask <- narrow the gap if it exists for too long try to get it to 100

// 2.
// If no one has posted any orders in 3 seconds, he will be a "market maker"
// He will sell a stock at $120 and immediately buy it back at $120

// discount by every second 0.5% or 0.05%.
// dividend on the stock 0.2% added to cash
