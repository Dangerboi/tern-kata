import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from urllib.error import HTTPError
from urllib.request import Request, urlopen


class Proxy(BaseHTTPRequestHandler):
    def forward(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        if self.command == "POST" and self.path == "/api/v1/projects/resolve":
            with open(sys.argv[2], "a", encoding="utf-8") as record:
                record.write(json.dumps(json.loads(body)) + "\n")
        headers = {key: value for key, value in self.headers.items()
                   if key.lower() not in ("host", "connection", "content-length")}
        request = Request(sys.argv[1].rstrip("/") + self.path,
                          data=body or None, headers=headers, method=self.command)
        try:
            response = urlopen(request)
        except HTTPError as error:
            response = error
        with response:
            self.send_response(response.status)
            for key, value in response.headers.items():
                if key.lower() not in ("transfer-encoding", "connection", "server", "date"):
                    self.send_header(key, value)
            self.end_headers()
            try:
                while line := response.readline():
                    self.wfile.write(line)
                    self.wfile.flush()
            except (BrokenPipeError, ConnectionResetError):
                pass

    do_GET = forward
    do_POST = forward

    def log_message(self, *_):
        pass


server = ThreadingHTTPServer(("127.0.0.1", 0), Proxy)
print(f"http://127.0.0.1:{server.server_port}", flush=True)
server.serve_forever()
