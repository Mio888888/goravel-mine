export default function mergeClassName(classList: string[], classMap: string | string[] | object): string {
  if (classMap !== null) {
    if (typeof classMap === 'string') {
      return [...classList, classMap].join(' ')
    }
    return [...classList, ...(classMap as string[])].join(' ')
  }
  return classList.join(' ')
}
