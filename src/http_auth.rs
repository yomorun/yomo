use axum::body::Body;
use axum::http::{Request, StatusCode, header};
use axum::middleware::Next;
use axum::response::{IntoResponse, Response};

pub async fn require_bearer_auth(
    axum::extract::State(auth_token): axum::extract::State<Option<String>>,
    request: Request<Body>,
    next: Next,
) -> Response {
    let Some(expected_token) = auth_token else {
        return next.run(request).await;
    };

    let auth_header = request
        .headers()
        .get(header::AUTHORIZATION)
        .and_then(|value| value.to_str().ok());

    match validate_bearer_token(auth_header, &expected_token) {
        Ok(()) => next.run(request).await,
        Err(message) => (
            StatusCode::UNAUTHORIZED,
            [(header::WWW_AUTHENTICATE, "Bearer")],
            message,
        )
            .into_response(),
    }
}

fn validate_bearer_token(auth_header: Option<&str>, expected_token: &str) -> Result<(), String> {
    let Some(auth_header) = auth_header else {
        return Err("missing Authorization header".to_string());
    };

    let value = auth_header.trim();
    if let Some(token) = value.strip_prefix("Bearer ") {
        if token == expected_token {
            return Ok(());
        }
        return Err("invalid bearer token".to_string());
    }

    Err("invalid Authorization format, expected 'Bearer <token>'".to_string())
}

#[cfg(test)]
mod tests {
    use super::validate_bearer_token;

    #[test]
    fn allows_matching_bearer_token() {
        let result = validate_bearer_token(Some("Bearer secret"), "secret");
        assert!(result.is_ok());
    }

    #[test]
    fn rejects_missing_header() {
        let result = validate_bearer_token(None, "secret");
        assert_eq!(
            result.expect_err("missing header should fail"),
            "missing Authorization header"
        );
    }

    #[test]
    fn rejects_invalid_scheme() {
        let result = validate_bearer_token(Some("Token secret"), "secret");
        assert_eq!(
            result.expect_err("invalid scheme should fail"),
            "invalid Authorization format, expected 'Bearer <token>'"
        );
    }

    #[test]
    fn rejects_invalid_token() {
        let result = validate_bearer_token(Some("Bearer wrong"), "secret");
        assert_eq!(
            result.expect_err("wrong token should fail"),
            "invalid bearer token"
        );
    }
}
