import {
  BoltIcon,
  CheckBadgeIcon,
  CodeBracketIcon,
  CurrencyDollarIcon,
  GlobeAltIcon,
  LockClosedIcon
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
    name: "Function Calling",
    description: `Improve LLM accuracy by function calling.`,
    link: "/docs/api/sfn",
    Icon: CheckBadgeIcon,
  },
  {
    name: "Low-latency",
    description: `Guaranteed by of QUIC Protocol and Streaming.`,
    link: "/docs/", //"https://datatracker.ietf.org/wg/quic/documents/",
    Icon: BoltIcon,
  },
  {
    name: "Cost Efficiency",
    description: `Less than $100 with 10K users with global scale.`,
    link: "/docs/devops_tuning",
    Icon: CurrencyDollarIcon,
  },
  {
    name: "Strongly-typed Language",
    description: `Write once, run on any model.`,
    link: "/docs/cli/build",
    Icon: CodeBracketIcon,
  },
  {
    name: "Security",
    description: `Every data packet encrypted with TLS v1.3.`,
    link: "/docs/devops_tls",
    Icon: LockClosedIcon,
  },
  {
    name: "Geo-distributed",
    description: `Run your model and tools close to your users.`,
    link: "/docs/glossary",
    Icon: GlobeAltIcon,
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
