export const description = 'this function is used for getting the weather'

export const tags = [0x33]

export type Argument = {
  unit: string;
  location: 'fahrenheit' | 'celsius';
}

async function getWeather(unit: string, location: string) {
  return { location: location, temperature: '22', unit: unit }
}
export async function handler(args: Argument) {
  const result = await getWeather(args.unit, args.location)
  return result
}