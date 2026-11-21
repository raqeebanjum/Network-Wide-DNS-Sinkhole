const dateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  second: '2-digit',
})

const compactDateTimeFormatter = new Intl.DateTimeFormat(undefined, {
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
})

const numberFormatter = new Intl.NumberFormat()

export function formatDateTime(value: string): string {
  return formatDate(value, dateTimeFormatter)
}

export function formatCompactDateTime(value: string): string {
  return formatDate(value, compactDateTimeFormatter)
}

export function formatNumber(value: number): string {
  return numberFormatter.format(value)
}

export function formatDuration(milliseconds: number): string {
  return `${numberFormatter.format(milliseconds)} ms`
}

function formatDate(value: string, formatter: Intl.DateTimeFormat): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) {
    return value
  }
  return formatter.format(date)
}
