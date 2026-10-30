{{flutter_js}}
{{flutter_build_config}}

// The Noto CJK fallback slices are self-hosted by the Go server at /fonts/;
// the engine default is a Google Fonts CDN.
_flutter.loader.load({
  config: {
    fontFallbackBaseUrl: "/fonts/",
  },
});
