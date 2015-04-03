$(document).ready(function() {
    var volumeData = [{label: 'Volume', values: []}];
    var volumeChart = $('#bar').epoch({
        type: 'time.bar',
        data: volumeData,
        axes: [],
    });
    
    var priceData = [{label: 'Price', values: []}];
    var priceChart = $('#line').epoch({
        type: 'time.line',
        data: priceData,
        axes: [],
    });

    // setInterval(function() {
    //     volumeChart.push([{time: new Date().getTime(), y: Math.random() * 100}]);
    //     priceChart.push([{time: new Date().getTime(), y: Math.random() * 100}]);
    // }, 500);
    var loc = window.location.host == "localhost:8008" ? "ws://localhost:8008/ws" : "ws://hft.hunterleath.com/ws";
    var trades = 0;
    var price = 0;
    var handlers = {
        filledOrder: [function(data) {
            // console.log("Filled Order", trades, price);
            trades += data.payload.quantity;
            price = data.payload.price;
        }],
        newOrder: [function() {}],
    };
    startSocket(loc, handlers);
    setInterval(function() {
        if(trades == 0) { return }
        // console.log("Adding data", trades, price)
        volumeChart.push([{time: new Date().getTime(), y: trades}]);
        priceChart.push([{time: new Date().getTime(), y: price}]);
        if(window.parent) {
            window.parent.postMessage({price: price}, "*");
        }
        trades = 0;
        price = 0;
    }, 500)
});

var startSocket = function(ws_uri, handlers) {
    var theSocket = new WebSocket(ws_uri);
    theSocket.onopen = function(event) {
        console.log("Opened websocket");
    }
    theSocket.onmessage = function(event) {
        var msg = JSON.parse(event.data);
        var typ = msg.type;
        console.log("got message", typ)
        if (handlers[typ] !== undefined) {
            try {
                for(var i in handlers[typ]) {
                    handlers[typ][i](msg);
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
}
