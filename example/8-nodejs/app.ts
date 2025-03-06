export const description = 'Get the current weather for `city_name`'

// For jsonschema in TypeScript, see: https://github.com/YousefED/typescript-json-schema
export type Argument = {
  /**
   * The name of the city to be queried
   */
  city_name: string;
}

async function getWeather(city_name: string) {
  console.log(`get weather for ${city_name}`)
  await sleep(3000)
  return { city_name: city_name, temperature: Math.floor(Math.random() * 41) }
}
export async function handler(args: Argument) {
  const result = await getWeather(args.city_name)
  return result
}

function sleep(ms: number) {
  return new Promise(resolve => setTimeout(resolve, ms));
}