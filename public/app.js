var tradingSimulator = angular.module('tradingSimulator', [
  'ngRoute',
  'tradingSimulatorControllers',
]);

tradingSimulator.config(['$routeProvider',
  function($routeProvider) {
    $routeProvider
      .when('/home', {
          controller: "HomeController",
          templateUrl: "partials/home.html",
      })
      .otherwise({
        redirectTo:'/home',
      })
}]);

tradingSimulator.directive('float', function(){
    return {
        require: 'ngModel',
        link: function(scope, ele, attr, ctrl){
            ctrl.$parsers.unshift(function(viewValue){
                return parseFloat(viewValue, 10);
            });
        }
    };
});

tradingSimulator.filter('prettyDate', function(d) {
    return moment(d).fromNow();
});
