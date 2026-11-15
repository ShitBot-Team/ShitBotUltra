let greeting = '你好';
exports.init = function (config) {
  greeting = config.greeting || '你好';
};
exports.onCommand = function (request) {
  return greeting + '，' + request.actor.display_name + '！';
};
