#!/usr/bin/env python3
"""Mock metrics generator simulating node-service /metrics endpoint.
Produces realistic Prometheus-format data for dashboard preview."""
import random
import time
from http.server import HTTPServer, BaseHTTPRequestHandler

# Persistent counters (increment over time)
_base_requests = 15000
_base_panics = 2
_base_doctor_pass = 340
_base_doctor_warn = 15
_base_doctor_fail = 3
_base_autofix = 28
_base_diag_sessions = 45
_base_exposure_applied = 12
_base_exposure_failed = 1
_base_grpc_msgs = 2000

class MetricsHandler(BaseHTTPRequestHandler):
    def do_GET(self):
        if self.path != '/metrics':
            self.send_response(404)
            self.end_headers()
            return

        t = int(time.time())
        # Add some randomness to counters
        req200 = _base_requests + random.randint(0, 50)
        req500 = random.randint(0, 5)
        rps = random.randint(50, 180)
        p99 = round(random.uniform(0.02, 0.45), 4)
        in_flight = random.randint(3, 25)
        grpc_conn = random.randint(1, 6)

        body = f"""# HELP http_requests_total Total HTTP requests by service and status.
# TYPE http_requests_total counter
http_requests_total{{service="api-gateway",status="200"}} {req200 + 8000}
http_requests_total{{service="api-gateway",status="500"}} {req500}
http_requests_total{{service="node-service",status="200"}} {req200}
http_requests_total{{service="node-service",status="500"}} {req500}
http_requests_total{{service="identity-service",status="200"}} {req200 + 3000}
http_requests_total{{service="identity-service",status="500"}} 0
http_requests_total{{service="subscription-service",status="200"}} {req200 + 1200}
http_requests_total{{service="subscription-service",status="500"}} 0
http_requests_total{{service="traffic-service",status="200"}} {req200 + 500}
http_requests_total{{service="traffic-service",status="500"}} 0
# HELP http_request_duration_seconds Request duration histogram.
# TYPE http_request_duration_seconds histogram
http_request_duration_seconds_bucket{{service="node-service",le="0.01"}} {int(rps*0.3)}
http_request_duration_seconds_bucket{{service="node-service",le="0.05"}} {int(rps*0.5)}
http_request_duration_seconds_bucket{{service="node-service",le="0.1"}} {int(rps*0.7)}
http_request_duration_seconds_bucket{{service="node-service",le="0.5"}} {int(rps*0.9)}
http_request_duration_seconds_bucket{{service="node-service",le="1.0"}} {rps}
http_request_duration_seconds_bucket{{service="node-service",le="+Inf"}} {rps}
http_request_duration_seconds_sum{{service="node-service"}} {round(p99*rps, 2)}
http_request_duration_seconds_count{{service="node-service"}} {rps}
http_request_duration_seconds_bucket{{service="api-gateway",le="0.01"}} {int(rps*1.5)}
http_request_duration_seconds_bucket{{service="api-gateway",le="0.05"}} {int(rps*2.5)}
http_request_duration_seconds_bucket{{service="api-gateway",le="0.1"}} {int(rps*3.5)}
http_request_duration_seconds_bucket{{service="api-gateway",le="0.5"}} {int(rps*4.5)}
http_request_duration_seconds_bucket{{service="api-gateway",le="1.0"}} {rps*5}
http_request_duration_seconds_bucket{{service="api-gateway",le="+Inf"}} {rps*5}
http_request_duration_seconds_sum{{service="api-gateway"}} {round(p99*rps*5, 2)}
http_request_duration_seconds_count{{service="api-gateway"}} {rps*5}
# HELP http_requests_in_flight Current in-flight requests.
# TYPE http_requests_in_flight gauge
http_requests_in_flight {in_flight}
# HELP http_panics_recovered_total Total panics recovered.
# TYPE http_panics_recovered_total counter
http_panics_recovered_total {_base_panics}
# HELP nodeservice_grpc_agent_connections Current active gRPC agent connections.
# TYPE nodeservice_grpc_agent_connections gauge
nodeservice_grpc_agent_connections {grpc_conn}
# HELP nodeservice_grpc_messages_received_total Total gRPC messages received.
# TYPE nodeservice_grpc_messages_received_total counter
nodeservice_grpc_messages_received_total{{message_type="heartbeat"}} {_base_grpc_msgs}
nodeservice_grpc_messages_received_total{{message_type="auth"}} {_base_grpc_msgs // 10}
nodeservice_grpc_messages_received_total{{message_type="config_result"}} {_base_grpc_msgs // 5}
# HELP nodeservice_grpc_messages_pushed_total Total gRPC messages pushed.
# TYPE nodeservice_grpc_messages_pushed_total counter
nodeservice_grpc_messages_pushed_total{{message_type="config_push"}} {_base_grpc_msgs // 8}
nodeservice_grpc_messages_pushed_total{{message_type="maintenance"}} {_base_autofix}
nodeservice_grpc_messages_pushed_total{{message_type="pong"}} {_base_grpc_msgs}
# HELP nodeservice_doctor_checks_total Total doctor checks by result.
# TYPE nodeservice_doctor_checks_total counter
nodeservice_doctor_checks_total{{result="pass"}} {_base_doctor_pass + random.randint(0, 5)}
nodeservice_doctor_checks_total{{result="warn"}} {_base_doctor_warn + random.randint(0, 2)}
nodeservice_doctor_checks_total{{result="fail"}} {_base_doctor_fail}
nodeservice_doctor_checks_total{{result="skip"}} 12
# HELP nodeservice_doctor_autofix_dispatched_total Total doctor autofix actions.
# TYPE nodeservice_doctor_autofix_dispatched_total counter
nodeservice_doctor_autofix_dispatched_total{{action="restart_kernel",status="dispatched"}} {_base_autofix}
nodeservice_doctor_autofix_dispatched_total{{action="reload_config",status="dispatched"}} 8
nodeservice_doctor_autofix_dispatched_total{{action="renew_cert",status="dispatched"}} 3
nodeservice_doctor_autofix_dispatched_total{{action="restart_kernel",status="failed"}} 1
# HELP nodeservice_channel_health_state Channel health state per server.
# TYPE nodeservice_channel_health_state gauge
nodeservice_channel_health_state{{server_id="sg01",active_channel="direct",state="healthy"}} 1
nodeservice_channel_health_state{{server_id="hk02",active_channel="cdn",state="degraded"}} 0
nodeservice_channel_health_state{{server_id="us03",active_channel="direct",state="healthy"}} 1
# HELP nodeservice_diagnosis_sessions_total Total AI diagnosis sessions.
# TYPE nodeservice_diagnosis_sessions_total counter
nodeservice_diagnosis_sessions_total{{category="config_error"}} 12
nodeservice_diagnosis_sessions_total{{category="network_issue"}} 8
nodeservice_diagnosis_sessions_total{{category="cert_expired"}} 5
nodeservice_diagnosis_sessions_total{{category="normal"}} 20
# HELP nodeservice_diagnosis_autofix_total Total AI diagnosis autofix actions.
# TYPE nodeservice_diagnosis_autofix_total counter
nodeservice_diagnosis_autofix_total{{action="restart_kernel",status="dispatched"}} 15
nodeservice_diagnosis_autofix_total{{action="reload_config",status="dispatched"}} 7
nodeservice_diagnosis_autofix_total{{action="renew_cert",status="dispatched"}} 3
nodeservice_diagnosis_autofix_total{{action="restart_kernel",status="failed"}} 1
# HELP nodeservice_exposure_applies_total Total exposure config applies.
# TYPE nodeservice_exposure_applies_total counter
nodeservice_exposure_applies_total{{status="applied"}} {_base_exposure_applied}
nodeservice_exposure_applies_total{{status="failed"}} {_base_exposure_failed}
"""
        self.send_response(200)
        self.send_header('Content-Type', 'text/plain; version=0.0.4')
        self.end_headers()
        self.wfile.write(body.encode())

    def log_message(self, *args):
        pass  # Suppress logs

if __name__ == '__main__':
    print("Mock metrics server running on :8082/metrics")
    HTTPServer(('0.0.0.0', 8082), MetricsHandler).serve_forever()
