import Document, { Head, Html, Main, NextScript } from 'next/document';
import { SkipNavLink } from "nextra-theme-docs";
import React from 'react';

class MyDocument extends Document {
  render() {
    return (
      <Html lang="en">
        <Head>
          {/* Global Site Tag (gtag.js) - Google Analytics */}
          <script
            async
            src={`https://www.googletagmanager.com/gtag/js?id=${process.env.NEXT_PUBLIC_GA_ID}`}
          />
          <script
            dangerouslySetInnerHTML={{
              __html: `
            window.dataLayer = window.dataLayer || [];
            function gtag(){dataLayer.push(arguments);}
            gtag('js', new Date());
            gtag('config', '${process.env.NEXT_PUBLIC_GA_ID}', {
              page_path: window.location.pathname,
            });
          `,
            }}
          />
          <script async src={`https://analytics.umami.is/script.js`} data-website-id={`d7fe47fa-d12e-42c4-b4ed-586bd3edf118`} />
          <link href="https://fonts.googleapis.com/css2?family=Exo+2&display=swap" rel="stylesheet"></link>
        </Head>
        <body>
          <SkipNavLink styled />
          <Main />
          <NextScript />
        </body>
      </Html>
    );
  }
}

export default MyDocument
