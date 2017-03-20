var http = require('http')

http.createServer(function (request, response) {
  response.writeHead(200)
  response.write('Hello there!\n')
  response.end()
}).listen(process.env.PORT || 3000)
