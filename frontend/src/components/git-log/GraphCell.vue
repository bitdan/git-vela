<script setup lang="ts">
import { useTemplateRef, watchPostEffect } from 'vue'
import type { GraphRow } from '../../../bindings/gitvela/internal/git'

const props = defineProps<{ row: GraphRow; width: number }>()
const canvas = useTemplateRef<HTMLCanvasElement>('canvas')
const palette = ['#6b9cff', '#45bdab', '#bb90ff', '#edaa59', '#f77699', '#5fb4eb', '#a0c969']
const height = 34
const x = (lane: number) => lane * 13 + 12

watchPostEffect(() => {
  const element = canvas.value
  const ctx = element?.getContext('2d')
  if (!element || !ctx) return
  const ratio = window.devicePixelRatio || 1
  element.width = Math.ceil(props.width * ratio)
  element.height = Math.ceil(height * ratio)
  ctx.scale(ratio, ratio)
  const row = props.row
  const mid = height / 2
  const line = (from: number, to: number, top: number, bottom: number, color: number) => {
    ctx.strokeStyle = palette[color % palette.length]
    ctx.lineWidth = 1.7
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
  ctx.arc(x(row.node), mid, 4.1, 0, Math.PI * 2)
  ctx.fill()
})
</script>

<template>
  <canvas ref="canvas" aria-hidden="true" :style="{ width: width + 'px', height: '34px' }" />
</template>
