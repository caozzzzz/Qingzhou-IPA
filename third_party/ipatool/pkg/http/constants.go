package http

type ResponseFormat string

const (
	ResponseFormatJSON ResponseFormat = "json"
	ResponseFormatRaw  ResponseFormat = "raw"
	ResponseFormatXML  ResponseFormat = "xml"
)

const DefaultUserAgent = "Configurator/2.21 (Macintosh; OS X 15.2; 24C5089c) AppleWebKit/0620.1.16.11.6"

// Apple Configurator authentication client profile used by IPATool 2.8.0.
const AuthenticationUserAgent = "Configurator/2.21 (Macintosh; OS X 27.0.1; 26A434) AppleWebKit/0625.2.5.11.6"

const CommerceUserAgent = DefaultUserAgent
