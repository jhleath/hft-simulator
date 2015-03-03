var maximumPrice = 140;
var minimumPrice = 60;
var generatePerson = function() {
    var startingCash = 2000;
    var startingShares = 10;
    var priceOfPurchasedShares = Math.random() * (maximumPrice - minimumPrice) + minimumPrice;
    priceOfPurchasedShares = Math.round(priceOfPurchasedShares * 100) / 100;

    return {
        cash: startingCash - (startingShares * priceOfPurchasedShares),
        stock: 10,
        initialPrice: priceOfPurchasedShares,
    }
}

var tradingSimulatorControllers = angular.module('tradingSimulatorControllers', []);

tradingSimulatorControllers.factory('tradeSocket', ['$rootScope', function($rootScope) {
    var loc = window.location, ws_uri;
    if (loc.protocol === "https:") {
        ws_uri = "wss:";
    } else {
        ws_uri = "ws:";
    }
    ws_uri += "//" + loc.host;
    ws_uri += loc.pathname + "ws";
    
    var theSocket = new WebSocket(ws_uri);
    theSocket.onopen = function(event) {
        console.log("Opened websocket");
    }
    theSocket.onmessage = function(event) {
        var msg = JSON.parse(event.data);
        var typ = msg.type;
        if (handlers[typ] !== undefined) {
            try {
                for(var i in handlers[typ]) {
                    $rootScope.$apply(function() {
                        handlers[typ][i](msg);
                    });
                }
            } catch (e) {
                console.log("Error calling handler...")
                console.log(e)
                console.dir(handlers[typ])
            }
        } else {
            console.log("Message delivered has no handler: " + typ);
            console.dir(msg);
        }
    }

    var handlers = {};

    return {
        registerHandler: function(typ, handler) {
            if(handlers[typ] == undefined) {
                handlers[typ] = [];
            }
            handlers[typ].push(handler);
        },
        send: function(msg) {
            theSocket.send(JSON.stringify(msg));
        }
    }
}]);

tradingSimulatorControllers.controller('HomeController', ['$scope', 'tradeSocket',
  function($scope, tradeSocket) {
      $scope.me = generatePerson()

      $scope.global = {
          traders: 1,
          shares: 10,
      }

      $scope.myOrders = [];

      $scope.lastPrice = $scope.me.initialPrice;
      $scope.buyBook = [];
      $scope.sellBook = [];

      tradeSocket.registerHandler("newOrder", function(data) {
          console.log("Got new order...")
          for(var i in $scope.myOrders) {
              if($scope.myOrders[i].id == data.payload.id) {
                  // This is our order!
                  console.log("Discarding own order")
                  return
              }
          }
          console.dir(data.payload);

          if(data.payload.sell) {
              $scope.sellBook.push(data.payload);
          } else {
              $scope.buyBook.push(data.payload);
          }
      })

      tradeSocket.registerHandler("filledOrder", function(data) {
          console.log("Filled some orders...")
          console.dir(data);

          var fillOrder = {
              date: new Date(),
              price: data.payload.price,
              quantity: data.payload.quantity,
          }
          $scope.history.push(fillOrder);

          var removeFromBooks = function(book, payload, compare, cb) {
              for(var i in book) {
                  if(compare(book[i].id, payload)) {
                      if(cb !== undefined) {
                          cb(book[i], payload.quantity, payload.price);
                      } else {
                          book[i].quantity -= payload.quantity
                      }

                      if(book[i].quantity == 0) {
                          book.splice(i, 1);
                      }

                      break;
                  }
              }
          }
          removeFromBooks($scope.buyBook, data.payload, function(bid, payload) { return bid == payload.buyOrder.id; });
          removeFromBooks($scope.sellBook, data.payload, function(bid, payload) { return bid == payload.sellOrder.id; });
          removeFromBooks($scope.myOrders, data.payload, function(bid, payload) {
              return bid == payload.buyOrder.id || bid == payload.sellOrder.id;
          }, function(order, quantity, price) {
              // One of our orders was fufilled
              console.log("One of our orders was partially fufilled!")

              if(order.sell) {
                  $scope.me.cash += price * quantity
              } else {
                  $scope.me.stock += quantity
              }
          });
          

          $scope.lastPrice = data.payload.price;
      })
      
      // {
      //     date: new Date(),
      //     price: 100, // strike price
      //     quantity: 1, //quantity
      // }
      $scope.history = [];

      var executeOrder = function(price, quantity, sell) {
          var order = {
              id: Math.random(),
              quantity: quantity,
              price: price,
              date: new Date(),
              sell: sell,
              me: true,
          }
          // Log it in our order books
          if(sell) {
              $scope.sellBook.push(order)
          } else {
              $scope.buyBook.push(order)
          }
          $scope.myOrders.push(order)

          // Send it to the server
          tradeSocket.send({
              order: order,
          })
      }
    
      $scope.buy = {
          price: $scope.lastPrice,
          quantity: 1,
      }
      $scope.executeBuy = function() {
          // console.log("Buying shares...");
          $scope.me.cash -= ($scope.buy.price * $scope.buy.quantity);
          executeOrder($scope.buy.price, $scope.buy.quantity, false);
          $scope.buy = {
              price: $scope.lastPrice,
              quantity: 1,
          }
      }

      $scope.sell = {
          price: $scope.lastPrice,
          quantity: 1,
      }
      $scope.executeSell = function() {
          // console.log("Selling shares...")
          $scope.me.stock -= $scope.sell.quantity;
          executeOrder($scope.sell.price, $scope.sell.quantity, true)
          $scope.sell = {
              price: $scope.lastPrice,
              quantity: 1,
          }
      }
}]);
