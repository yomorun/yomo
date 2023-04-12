import {
  BoltIcon,
  GlobeAltIcon,
  GlobeAsiaAustraliaIcon,
  CodeBracketIcon,
  CurrencyDollarIcon,
  LockClosedIcon,
  WifiIcon,
} from "@heroicons/react/24/outline";
import { ComponentProps } from "react";

export type Feature = {
  name: string;
  description: string;
  Icon: (props: ComponentProps<"svg">) => JSX.Element;
  // page: "all" | "home" | "docs";
};

export type Features = Array<Feature>;

const FEATURES: Features = [
  {
    name: "Low-latency",
    description: `Guaranteed by implementing atop of
    [QUIC](https://datatracker.ietf.org/wg/quic/documents/)`,
    Icon: BoltIcon,
  },
  {
    name: "Security",
    description: `TLS v1.3 on every data packet by design.`,
    Icon: LockClosedIcon,
  },
  {
    name: "Open source",
    description: `See Github.`,
    Icon: CodeBracketIcon,
  },
  {
    name: "Geo-distributed Architecture",
    description: `Your code close to your user.`,
    Icon: GlobeAltIcon,
  },
  {
    name: "5G/WiFi-6",
    description: `Reliable networking in Celluar/Wireless.`,
    Icon: WifiIcon,
  },
  {
    name: "Streaming Serverless",
    description: `Stateful serverless.`,
    Icon: CurrencyDollarIcon,
  },
];

export default function FeatureList() {
  return (
    <div className="grid grid-cols-1 mt-12 gap-x-6 gap-y-12 sm:grid-cols-2 lg:mt-16 lg:grid-cols-3 lg:gap-x-8 lg:gap-y-12">
      {FEATURES.map((feature) => (
        <div className="p-10 bg-white shadow-lg rounded-xl dark:bg-opacity-5" key={feature.name.split(" ").join("-")}>
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
        </div>
      ))}
    </div>
  );
}
