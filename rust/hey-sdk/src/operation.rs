use std::borrow::Cow;
use std::fmt::Display;

use bytes::Bytes;
use reqwest::Method;
use serde::Serialize;
use url::Url;

use crate::error::Error;
use crate::observability::OperationInfo;
use crate::route::Route;

/// A request the client has not sent yet. Generated service methods build one from a
/// [`Route`]; [`crate::Client::request`] builds one for anything the model does not cover.
#[derive(Debug, Clone)]
pub struct Operation {
    pub(crate) id: Cow<'static, str>,
    pub(crate) info: OperationInfo,
    pub(crate) method: Method,
    pub(crate) path: String,
    pub(crate) url: Option<Url>,
    pub(crate) query: Vec<(String, String)>,
    pub(crate) body: Option<Body>,
    pub(crate) idempotent: bool,
    pub(crate) empty_on: &'static [u16],
    pub(crate) accept: &'static str,
    /// HEY answers JSON to paths that end in `.json`, so a modelled route gets one put
    /// back on. A raw path is sent as the caller wrote it.
    pub(crate) json_suffix: bool,
    pub(crate) no_cache: bool,
    pub(crate) capture_redirects: bool,
    /// One request inside another operation rather than an operation of its own. See
    /// [`Operation::quiet`].
    pub(crate) quiet: bool,
}

#[derive(Debug, Clone)]
pub(crate) struct Body {
    pub(crate) content_type: String,
    pub(crate) bytes: Bytes,
}

/// The two redirects HEY's form-backed endpoints answer with, which
/// [`Operation::capture_redirects`] takes for an answer rather than a failure.
const REDIRECTS: &[u16] = &[302, 303];

impl Operation {
    pub(crate) fn for_route(route: &'static Route, params: &[&dyn Display]) -> Operation {
        Operation {
            id: Cow::Borrowed(route.id),
            info: OperationInfo {
                service: Cow::Borrowed(route.service),
                operation: Cow::Borrowed(route.id),
                resource_type: Cow::Borrowed(route.resource_type),
                is_mutation: !route.readonly,
                resource_id: None,
            },
            method: route.method.clone(),
            path: route.fill(params),
            url: None,
            query: Vec::new(),
            body: None,
            idempotent: route.idempotent,
            empty_on: route.empty_on,
            accept: "application/json",
            json_suffix: true,
            no_cache: false,
            capture_redirects: false,
            quiet: false,
        }
    }

    pub(crate) fn raw(method: Method, path: String) -> Operation {
        let idempotent = matches!(
            method,
            Method::GET | Method::HEAD | Method::PUT | Method::DELETE
        );
        let id = format!("{method} {path}");
        let info = OperationInfo {
            service: Cow::Borrowed("Raw"),
            operation: Cow::Owned(id.clone()),
            resource_type: Cow::Borrowed("raw"),
            is_mutation: method != Method::GET,
            resource_id: None,
        };
        Operation {
            id: Cow::Owned(id),
            info,
            method,
            path,
            url: None,
            query: Vec::new(),
            body: None,
            idempotent,
            empty_on: &[],
            accept: "application/json",
            json_suffix: true,
            no_cache: false,
            capture_redirects: false,
            quiet: false,
        }
    }

    pub(crate) fn at(method: Method, url: Url) -> Operation {
        let mut operation = Operation::raw(method, url.path().to_string());
        operation.url = Some(url);
        operation
    }

    pub fn id(&self) -> &str {
        &self.id
    }

    pub fn method(&self) -> &Method {
        &self.method
    }

    pub fn path(&self) -> &str {
        &self.path
    }

    pub fn query(&mut self, name: &str, value: impl Display) -> &mut Operation {
        self.query.push((name.to_string(), value.to_string()));
        self
    }

    pub fn query_optional<T: Display>(&mut self, name: &str, value: Option<&T>) -> &mut Operation {
        if let Some(value) = value {
            self.query(name, value);
        }
        self
    }

    pub fn json<T: Serialize + ?Sized>(&mut self, body: &T) -> Result<&mut Operation, Error> {
        self.body_bytes("application/json", Bytes::from(serde_json::to_vec(body)?));
        Ok(self)
    }

    pub fn form(&mut self, fields: &[(&str, &str)]) -> &mut Operation {
        let encoded = url::form_urlencoded::Serializer::new(String::new())
            .extend_pairs(fields)
            .finish();
        self.body_bytes("application/x-www-form-urlencoded", Bytes::from(encoded))
    }

    /// A multipart body the caller assembled, boundary and all. The content type has to
    /// name that same boundary for HEY to read the parts.
    pub fn multipart(&mut self, content_type: String, body: Bytes) -> &mut Operation {
        self.body_bytes(content_type, body)
    }

    /// A body the caller encoded, for the representations the model does not describe.
    pub fn body_bytes(&mut self, content_type: impl Into<String>, bytes: Bytes) -> &mut Operation {
        self.body = Some(Body {
            content_type: content_type.into(),
            bytes,
        });
        self
    }

    /// Replaces the whole of what the operation announces itself as to the client's
    /// [`crate::observability::Hooks`].
    pub fn info(&mut self, info: OperationInfo) -> &mut Operation {
        self.info = info;
        self
    }

    /// Announces the operation as something other than the route it sends: HEY stops a
    /// time track by updating one, and a wrapper that does so says `StopTimeTrack`.
    pub fn operation_name(&mut self, name: impl Into<Cow<'static, str>>) -> &mut Operation {
        self.info.operation = name.into();
        self
    }

    pub fn resource_type(&mut self, resource_type: impl Into<Cow<'static, str>>) -> &mut Operation {
        self.info.resource_type = resource_type.into();
        self
    }

    /// Names the record the operation acts on. Generated methods set this from the path
    /// parameter that names it.
    pub fn resource_id(&mut self, resource_id: i64) -> &mut Operation {
        self.info.resource_id = Some(resource_id);
        self
    }

    /// Marks the operation as safe to resend, or not, regardless of its HTTP method.
    pub fn idempotent(&mut self, idempotent: bool) -> &mut Operation {
        self.idempotent = idempotent;
        self
    }

    pub fn accept(&mut self, media_type: &'static str) -> &mut Operation {
        self.accept = media_type;
        self
    }

    /// Sends the path as it stands. A modelled route gets a `.json` suffix put back on the
    /// paths Smithy cannot spell it into; a path the caller wrote needs no such repair.
    pub fn without_json_suffix(&mut self) -> &mut Operation {
        self.json_suffix = false;
        self
    }

    /// Reads past the response cache for this send, so the answer is HEY's own. A blob is
    /// read this way: the cache is for JSON documents.
    pub fn no_cache(&mut self) -> &mut Operation {
        self.no_cache = true;
        self
    }

    /// Treats a redirect as the answer rather than following it. HEY's form-backed
    /// endpoints answer a 302 or 303 naming what they created, and that `Location` is the
    /// whole of what the request was for.
    pub fn capture_redirects(&mut self) -> &mut Operation {
        self.capture_redirects = true;
        self.empty_on = REDIRECTS;
        self
    }

    /// Asks for the HTML representation a form-backed endpoint serves: no `.json` suffix,
    /// and no preference about what comes back.
    pub fn form_representation(&mut self) -> &mut Operation {
        self.without_json_suffix().accept("*/*")
    }

    /// Sends this without announcing an operation: no gate, no start, no end. For a request
    /// made inside another operation — the read-back a write needs to answer with the record
    /// it wrote — so the hooks hear about that operation once rather than twice.
    ///
    /// The request hooks still fire, so every send the SDK makes is still reported. So is
    /// every layer the announced operation went through: a quiet send is inside its
    /// bulkhead permit and under its circuit breaker, not beside them.
    pub fn quiet(&mut self) -> &mut Operation {
        self.quiet = true;
        self
    }
}
