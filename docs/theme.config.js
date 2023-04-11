import { useRouter } from "next/router";
import { useConfig } from "nextra-theme-docs";
import Logo from "./components/allegro";
// import { Discord, Github } from "./components/social";

/** @type {import('nextra-theme-docs').DocsThemeConfig} */
const themeConfig = {
  sidebar: {
    defaultMenuCollapseLevel: 3,
  },
  project: {
    link: "https://github.com/yomorun/yomo",
  },
  docsRepositoryBase: "https://github.com/yomorun/yomo/tree/master/docs",
  useNextSeoProps() {
    return {
      titleTemplate: "%s – YoMo",
    };
  },
  toc: {
    float: true,
    title: "On this page",
  },
  search: {
    placeholder: "Search documentation ...",
  },
  editLink: {
    text: "edit this page",
  },
  feedback: {
    content: "feedback",
  },
  chat: {
    link: 'https://discord.gg/Ugam5qAvHy',
  },
  navbar: {
    // extraContent: (
    //   <>
    //     <Github />
    //     <Discord />
    //   </>
    // ),
  },
  nextThemes: {
  },
  logo: () => {
    return (
      <>
        <Logo height={24} />
        <span
          className="mx-2 font-extrabold hidden md:inline select-none"
          title="Allegro Cloud"
        >
          YoMo
        </span>
      </>
    );
  },
  head: () => {
    const { route } = useRouter();
    const { frontMatter, title } = useConfig();
    const titleSuffix = "Tutorials";
    const description = "Edge Infra for geo-distributed applications"

    const imageUrl = new URL("https://yomo.run/api/og"); // TODO

    if (!/\/index\.+/.test(route)) {
      imageUrl.searchParams.set("title", title || titleSuffix);
    }

    const ogTitle = title ? `${title} – YoMo` : `YoMo: ${titleSuffix}`;
    const ogDescription = frontMatter.description || description;
    const ogImage = frontMatter.image || imageUrl.toString();

    return (
      <>
        {/* Favicons, meta */}
        <link rel="icon" type="image/svg+xml" href="/favicon/favicon.svg" />
        <link rel="manifest" href="/favicon/site.webmanifest" />
        <link rel="mask-icon" href="/favicon/safari-pinned-tab.svg" color="#000000" />
        <meta httpEquiv="Content-Language" content="en-US" />
        <meta name="msapplication-TileColor" content="#ffffff" />
        <meta name="apple-mobile-web-app-title" content="SWR" />
        <meta name="description" content={ogDescription} />
        <meta name="twitter:card" content="summary_large_image" />
        <meta name="twitter:site" content="@vercel" />
        <meta name="twitter:image" content={ogImage} />
        <meta property="og:title" content={ogTitle} />
        <meta property="og:description" content={ogDescription} />
        <meta property="og:image" content={ogImage} />
        <link rel="preconnect" href="https://fonts.googleapis.com"></link>
        <link rel="preconnect" href="https://fonts.gstatic.com" crossOrigin="crossorigin"></link>
        <link href="https://fonts.googleapis.com/css2?family=Exo+2&display=swap" rel="stylesheet"></link>
        <meta name="msvalidate.01" content="" />
        <meta name="google-site-verification" content="" />
      </>
    );
  },
  footer: {
    text: () => {
      return (
        <div>
          <a
            href={`https://allegrocloud.io/?utm_source=YoMo-doc`}
            target="_blank"
            rel="noopener"
            className="inline-flex items-center no-underline text-current font-semibold"
          >
            <span>
              <Logo height={20} />
            </span>
          </a>
          <a href={`https://vercel.com/?utm_source=yomorun&utm_campaign=oss`} target="_blank" rel="noopener"><img src="/vercel.svg" /></a>
        </div>
      );
    },
  },
};

export default themeConfig;
