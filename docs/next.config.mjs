import { remarkMermaid } from 'remark-mermaid-nextra';
import nextra from 'nextra';

//const withNextra = require("nextra")
const withNextra = nextra({
  theme: "nextra-theme-docs",
  themeConfig: "./theme.config.js",
  staticImage: true,
  flexsearch: {
    codeblocks: false,
  },
  defaultShowCopyCode: true,
  mdxOptions: {
    remarkPlugins: [remarkMermaid]
  },
});

export default withNextra({
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
