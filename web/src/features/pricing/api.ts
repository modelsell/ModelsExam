// No pricing catalog: the model field is free text.
export async function getPricing(): Promise<{
  data: { model_name: string }[]
}> {
  return { data: [] }
}
