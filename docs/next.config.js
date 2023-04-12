const withNextra = require("nextra")({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js",
  staticImage: true,
  flexsearch: {
    codeblocks: false,
  },
  defaultShowCopyCode: true,
});

module.exports = withNextra({
  images: {
    unoptimized: true,
  },
  redirects: () => {
    return [
      {
        source: "/cc",
        destination: "/docs/cc",
        statusCode: 301,
      },
    ];
  },
  reactStrictMode: false,
});
