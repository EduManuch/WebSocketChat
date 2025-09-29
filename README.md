# gowebsockets

WebSocketChat

[RFC 6455](https://datatracker.ietf.org/doc/html/rfc6455)

[Gorilla](https://github.com/gorilla/websocket)

[Project Layout](https://github.com/golang-standards/project-layout)

# TLS
***Example of Installing self-signed certificate and CA***
```
sudo apt install mkcert
mkcert -install
mkcert 127.0.0.1
```

[Add Certificate to Ubuntu and Firefox](https://linuxconfig.org/step-by-step-guide-adding-certificates-to-ubuntus-trusted-authorities)

Change in index.html ```WebSocket('ws://...);``` to ```new WebSocket('wss://...);```
