import os
from datetime import datetime, timezone

import redis
from microdot import Microdot, Response
from microdot.jinja import Template

app = Microdot()
Response.default_content_type = "text/html"

# Redis connection
redis_client = redis.Redis(
    host=os.getenv("REDIS_HOST", "localhost"),
    port=int(os.getenv("REDIS_PORT", 6379)),
    password=os.getenv("REDIS_PASSWORD", ""),
    db=int(os.getenv("REDIS_DB", 0)),
    decode_responses=True,
)


def get_all_endpoints():
    """Get all unique endpoints from Redis"""
    endpoints = set()

    # Get all status keys
    for key in redis_client.scan_iter("status:*"):
        endpoint = key.replace("status:", "")
        endpoints.add(endpoint)

    # Get all ssl keys
    for key in redis_client.scan_iter("ssl:*"):
        endpoint = key.replace("ssl:", "")
        endpoints.add(endpoint)

    return list(endpoints)


def get_endpoint_data(endpoint):
    """Get all data for a specific endpoint"""
    data = {
        "endpoint": endpoint,
        "status": None,
        "status_code": 0,
        "ssl_expiration": None,
        "days_left": None,
        "last_status_update": None,
        "last_ssl_update": None,
        "is_https": endpoint.startswith("https://"),
    }

    # Get HTTP status
    status_key = f"status:{endpoint}"
    status = redis_client.get(status_key)
    if status:
        try:
            data["status_code"] = int(status)
            data["status"] = status
        except ValueError:
            data["status"] = status

    # Get status update time
    status_updated_key = f"status_updated:{endpoint}"
    status_updated = redis_client.get(status_updated_key)
    if status_updated:
        try:
            timestamp = int(status_updated)
            data["last_status_update"] = datetime.fromtimestamp(
                timestamp, tz=timezone.utc
            )
        except (ValueError, OSError):
            pass

    # Get SSL expiration (only for HTTPS)
    if data["is_https"]:
        ssl_key = f"ssl:{endpoint}"
        ssl_exp = redis_client.get(ssl_key)
        if ssl_exp:
            try:
                exp_timestamp = int(ssl_exp)
                exp_date = datetime.fromtimestamp(exp_timestamp, tz=timezone.utc)
                data["ssl_expiration"] = exp_date

                # Calculate days left
                now = datetime.now(timezone.utc)
                delta = exp_date - now
                data["days_left"] = delta.days
            except (ValueError, OSError):
                pass

        # Get SSL update time
        ssl_updated_key = f"ssl_updated:{endpoint}"
        ssl_updated = redis_client.get(ssl_updated_key)
        if ssl_updated:
            try:
                timestamp = int(ssl_updated)
                data["last_ssl_update"] = datetime.fromtimestamp(
                    timestamp, tz=timezone.utc
                )
            except (ValueError, OSError):
                pass

    return data


def get_status_class(status_code):
    """Return CSS class based on HTTP status code"""
    if status_code == 0:
        return "status-error"
    elif 200 <= status_code < 300:
        return "status-success"
    elif 300 <= status_code < 400:
        return "status-redirect"
    elif 400 <= status_code < 500:
        return "status-client-error"
    elif 500 <= status_code < 600:
        return "status-server-error"
    else:
        return "status-unknown"


def get_ssl_class(days_left):
    """Return CSS class based on days left for SSL expiration"""
    if days_left is None:
        return ""
    elif days_left < 0:
        return "ssl-expired"
    elif days_left < 7:
        return "ssl-critical"
    elif days_left < 30:
        return "ssl-warning"
    else:
        return "ssl-ok"


def format_time_ago(dt):
    """Format datetime as 'X minutes/hours/days ago'"""
    if dt is None:
        return "Never"

    now = datetime.now(timezone.utc)
    delta = now - dt

    seconds = delta.total_seconds()
    if seconds < 60:
        return f"{int(seconds)}s ago"
    elif seconds < 3600:
        return f"{int(seconds / 60)}m ago"
    elif seconds < 86400:
        return f"{int(seconds / 3600)}h ago"
    else:
        return f"{int(seconds / 86400)}d ago"


def prepare_endpoint_display_data(data):
    """Prepare endpoint data for display in template"""
    display_data = data.copy()

    # Add CSS classes
    display_data["status_class"] = get_status_class(data["status_code"])
    display_data["ssl_class"] = get_ssl_class(data["days_left"])

    # Format status text
    display_data["status_text"] = (
        str(data["status_code"]) if data["status_code"] else "N/A"
    )

    # Format SSL text
    if data["is_https"]:
        if data["days_left"] is not None:
            if data["days_left"] < 0:
                display_data["ssl_text"] = f"Expired {abs(data['days_left'])} days ago"
            else:
                display_data["ssl_text"] = f"{data['days_left']} days left"
        else:
            display_data["ssl_text"] = "Checking..."
    else:
        display_data["ssl_text"] = "HTTP only"

    # Format last update time
    last_update = data["last_status_update"] or data["last_ssl_update"]
    display_data["update_text"] = format_time_ago(last_update)

    return display_data


@app.route("/")
def index(request):
    """Main dashboard page"""
    endpoints = get_all_endpoints()

    # Get data for all endpoints
    endpoint_data = []
    for endpoint in endpoints:
        data = get_endpoint_data(endpoint)
        endpoint_data.append(data)

    # Sort by SSL expiration days (None values go to the end)
    endpoint_data.sort(
        key=lambda x: (
            x["days_left"] is None,
            x["days_left"] if x["days_left"] is not None else float("inf"),
        )
    )

    # Prepare display data
    display_endpoints = [prepare_endpoint_display_data(data) for data in endpoint_data]

    # Calculate statistics
    total_endpoints = len(endpoint_data)
    healthy_count = sum(
        1 for e in endpoint_data if e["status_code"] and 200 <= e["status_code"] < 300
    )
    ssl_warning_count = sum(
        1 for e in endpoint_data if e["days_left"] is not None and e["days_left"] < 30
    )

    # Current time
    current_time = datetime.now(timezone.utc).strftime("%H:%M:%S UTC")

    return Template("index.html").render(
        endpoints=display_endpoints,
        total_endpoints=total_endpoints,
        healthy_count=healthy_count,
        ssl_warning_count=ssl_warning_count,
        current_time=current_time,
    )


@app.route("/api/endpoints")
def api_endpoints(request):
    """JSON API endpoint for programmatic access"""
    endpoints = get_all_endpoints()

    endpoint_data = []
    for endpoint in endpoints:
        data = get_endpoint_data(endpoint)
        # Convert datetime objects to ISO format strings
        if data["last_status_update"]:
            data["last_status_update"] = data["last_status_update"].isoformat()
        if data["last_ssl_update"]:
            data["last_ssl_update"] = data["last_ssl_update"].isoformat()
        if data["ssl_expiration"]:
            data["ssl_expiration"] = data["ssl_expiration"].isoformat()

        endpoint_data.append(data)

    endpoint_data.sort(
        key=lambda x: (
            x["days_left"] is None,
            x["days_left"] if x["days_left"] is not None else float("inf"),
        )
    )

    return {"endpoints": endpoint_data, "total": len(endpoint_data)}


if __name__ == "__main__":
    print("Starting Endpoint Monitor Dashboard...")
    print("Access the dashboard at: http://localhost:5000")
    app.run(host="0.0.0.0", port=5000, debug=True)
