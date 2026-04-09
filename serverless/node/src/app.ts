export const description = "Get weather for a city"

export type Argument = {
    /**
     * The city name to get the weather for.
     */
    city: string
}

export async function handler(args: Argument): Promise<string> {
    const city = (args.city || "").trim()
    if (!city) {
        throw new Error("city is required")
    }

    console.log(`query weather for city: ${city}`)

    const url = `https://wttr.in/${encodeURIComponent(city)}?format=3`
    const resp = await fetch(url)
    if (!resp.ok) {
        throw new Error(`failed to query weather, status code: ${resp.status}`)
    }

    const result = await resp.text()
    console.log(result)

    return result
}
