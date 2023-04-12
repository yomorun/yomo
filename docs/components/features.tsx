import {
  BoltIcon,
  GlobeAltIcon,
  CodeBracketIcon,
  CurrencyDollarIcon,
  LockClosedIcon,
  WifiIcon,
} from "@heroicons/react/24/outline";
import { ComponentProps } from "react";

export type Feature = {
  name: string;
  description: string;
  link: string;
  Icon: (props: ComponentProps<"svg">) => JSX.Element;
};

export type Features = Array<Feature>;

const FEATURES: Features = [
  {
    name: "Low-latency",
    description: `Guaranteed by implementing atop of QUIC Protocol.`,
    link: "/docs/", //"https://datatracker.ietf.org/wg/quic/documents/",
    Icon: BoltIcon,
  },
  {
    name: "Geo-distributed",// Architecture",
    description: `Your code close to your users.`,
    link: "/docs/geo-distributed",
    Icon: GlobeAltIcon,
  },
  {
    name: "Self-hosting",// Efficiency",
    description: `Less than $100 with 10K users with global scale.`,
    link: "/docs/hosting",
    Icon: CurrencyDollarIcon,
  },
  {
    name: "WebAssembly",
    description: `Implement serverless function in Rust / Go / C, compile to wasm, run it everywhere.`,
    link: "/docs/sfn-wasm",
    Icon: CodeBracketIcon,
  },
  {
    name: "Security",
    description: `Every data packet encrypted by TLS v1.3.`,
    link: "/docs/security",
    Icon: LockClosedIcon,
  },
  {
    name: "5G/WiFi-6",
    description: `Networking optimized for high-speed celluar and wireless.`,
    link: "/docs/performance",
    Icon: WifiIcon,
  },
];

export default function FeatureList() {
  return (
    <div className="grid grid-cols-1 mt-12 gap-x-6 gap-y-12 sm:grid-cols-2 lg:mt-16 lg:grid-cols-3 lg:gap-x-8 lg:gap-y-12">
      {FEATURES.map((feature) => (
        <div className="p-10 bg-white shadow-lg rounded-xl dark:bg-opacity-5" key={feature.name.split(" ").join("-")}>
          <a href={feature.link}>
            <div>
              <feature.Icon
                className="h-8 w-8 dark:text-white rounded-full p-1.5 dark:bg-white dark:bg-opacity-10 bg-black bg-opacity-5 text-black"
                aria-hidden="true"
              />
            </div>
            <div className="mt-4">
              <h3 className="text-lg font-medium dark:text-white">{feature.name}</h3>
              <p className="mt-2 text-base font-medium text-gray-500 dark:text-gray-400">
                {feature.description}
              </p>
            </div>
          </a>
        </div>
      ))}
    </div>
  );
}
