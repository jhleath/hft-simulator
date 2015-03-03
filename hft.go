package main

import (
	"fmt"
	"math/rand"
	"time"
)

type simpleTrader struct {
	Cash        float64
	Stock       int
	Mean        float64
	Outstanding map[float64]*order
}

func newSimpleTrader(mean float64) *simpleTrader {
	t := &simpleTrader{
		Mean:        mean,
		Outstanding: make(map[float64]*order),
	}

	connectionChannel <- &connectionInfo{
		trader: t,
		Open:   true,
	}

	return t
}

func (s *simpleTrader) writeMessage(typ string, data interface{}) {
	if typ == "newOrder" {
		s.newOrder(data.(*order))
	} else if typ == "cancelledOrder" {

	} else if typ == "filledOrder" {
		m := data.(map[string]interface{})
		s.filledOrder(
			m["sellOrder"].(*order),
			m["buyOrder"].(*order),
			m["quantity"].(int),
			m["price"].(float64),
		)
	}
}

func (s *simpleTrader) placeOrder(o *order) {
	s.Outstanding[o.ID] = o
	go func() {
		listChannel <- o
	}()
}

func (s *simpleTrader) filledOrder(sell *order, buy *order, q int, p float64) {
	if _, sok := s.Outstanding[sell.ID]; sok {
		s.Cash += float64(q) * p
		delete(s.Outstanding, sell.ID)
	} else if _, bok := s.Outstanding[buy.ID]; bok {
		s.Stock += q
		delete(s.Outstanding, buy.ID)
	}

	fmt.Println("Trader Update", s.Cash, "dollars", s.Stock, "shares")
}

func (s *simpleTrader) newOrder(o *order) {
	if (o.Sell && o.Price < s.Mean) || (!o.Sell && o.Price > s.Mean) {
		newOrder := &order{
			ID:       rand.Float64(),
			Quantity: o.Quantity,
			Price:    o.Price,
			Date:     time.Now(),
			Sell:     !o.Sell,
			Owner:    s,
		}
		s.placeOrder(newOrder)
		if newOrder.Sell {
			s.Stock -= o.Quantity
		} else {
			s.Cash -= float64(o.Quantity) * o.Price
		}

		s.Outstanding[newOrder.ID] = newOrder
	}
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

// 2.
// If no one has posted any orders in 3 seconds, he will be a "market maker"
// He will sell a stock at $120 and immediately buy it back at $120
