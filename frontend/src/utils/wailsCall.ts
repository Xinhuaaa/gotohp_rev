import { Call } from '@wailsio/runtime'

export async function callByAnyName<T>(methodNames: string[], ...args: any[]): Promise<T> {
  let lastError: any = null

  for (const methodName of methodNames) {
    try {
      return (await Call.ByName(methodName, ...args)) as T
    } catch (error: any) {
      lastError = error
      const message = error?.message ? String(error.message) : String(error)
      if (message.includes("unknown bound method name")) {
        continue
      }
      throw error
    }
  }

  throw lastError || new Error(`All method names failed: ${methodNames.join(', ')}`)
}

