<script setup lang="ts">
import type { PermissionCode } from "@/shared/permissions";
import { usePermission } from "@/shared/permissions";

const props = defineProps<{
  permission?: PermissionCode | PermissionCode[];
}>();

const { allowed } = usePermission(props.permission);
</script>

<template>
  <slot v-if="allowed" />
  <slot v-else name="fallback" />
</template>
