<script setup lang="ts">
import { useTemplateRef, watchPostEffect } from 'vue'
import { usePreferences } from '../../stores/preferences'
import type { GraphRow } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ row: GraphRow; width: number; height: number }>()
const canvas = useTemplateRef<HTMLCanvasElement>('canvas')
const preferences = usePreferences()
const lightPalette = ['#326cc2', '#27866b', '#805db8', '#ae6a2c', '#ab5279', '#397f9d', '#618532']
const darkPalette = ['#86b0f1', '#71bfa7', '#b79be0', '#d4a067', '#db91ad', '#71b4cb', '#a7c375']
const x = (lane: number) => lane * 13 + 12

watchPostEffect(() => {
  const element = canvas.value
  const ctx = element?.getContext('2d')
  if (!element || !ctx) return
  const height = props.height
  const ratio = window.devicePixelRatio || 1
  element.width = Math.ceil(props.width * ratio)
  element.height = Math.ceil(height * ratio)
  ctx.scale(ratio, ratio)
  const row = props.row
  const palette = preferences.dark ? darkPalette : lightPalette
  const mid = height / 2
  const line = (from: number, to: number, top: number, bottom: number, color: number) => {
    ctx.strokeStyle = palette[color % palette.length]
    ctx.lineWidth = 1.5
    ctx.beginPath()
    ctx.moveTo(x(from), top)
    if (from === to) ctx.lineTo(x(to), bottom)
    else ctx.bezierCurveTo(x(from), (top + bottom) / 2, x(to), (top + bottom) / 2, x(to), bottom)
    ctx.stroke()
  }
  for (const edge of row.through ?? []) line(edge.from, edge.to, 0, height, edge.color)
  if (row.incoming) line(row.node, row.node, 0, mid, row.color)
  for (const edge of row.out ?? []) line(edge.from, edge.to, mid, height, edge.color)
  ctx.fillStyle = palette[row.color % palette.length]
  ctx.beginPath()
  ctx.arc(x(row.node), mid, 3.8, 0, Math.PI * 2)
  ctx.fill()
})
</script>

<template>
  <canvas ref="canvas" aria-hidden="true" :style="{ width: width + 'px', height: height + 'px' }" />
</template>
