import { setupDevPlatform } from "@cloudflare/next-on-pages/next-dev";
import nextra from 'nextra';
import { remarkMermaid } from 'remark-mermaid-nextra';

if (process.env.NODE_ENV === "development") {  await setupDevPlatform()
}

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
        source: "/prscd",
        destination: "/prscd.html",
        statusCode: 301,
      },
    ];
  },
  reactStrictMode: false,
});
