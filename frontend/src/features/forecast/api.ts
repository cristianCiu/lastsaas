import api from '../../api/client'; import type { ForecastCoverage,ForecastMaturity,ForecastPoint,ForecastPolicy,ForecastRecommendation,ForecastRun,GuestPlan } from './types';
const base=(locationId:string)=>`/product/locations/${locationId}/forecast`;
export const forecastApi={
 guestPlans:(locationId:string)=>api.get<{guestPlans:GuestPlan[]}>(`${base(locationId)}/guest-plans`).then(r=>r.data),
 createGuestPlan:(locationId:string,input:{planDate:string;servicePeriod:string;guestCount:number;notes?:string})=>api.post<{guestPlan:GuestPlan}>(`${base(locationId)}/guest-plans`,input).then(r=>r.data),
 policies:(locationId:string)=>api.get<{forecastPolicies:ForecastPolicy[]}>(`${base(locationId)}/policies`).then(r=>r.data),
 createPolicy:(locationId:string,input:{name:string;horizonDays:number;lookbackDays:number;safetyStockDays:number;isActive:boolean})=>api.post<{forecastPolicy:ForecastPolicy}>(`${base(locationId)}/policies`,input).then(r=>r.data),
 updatePolicy:(locationId:string,id:string,input:{version:number;name?:string;horizonDays?:number;lookbackDays?:number;safetyStockDays?:number;isActive?:boolean})=>api.patch<{forecastPolicy:ForecastPolicy}>(`${base(locationId)}/policies/${id}`,input).then(r=>r.data),
 datasets:(locationId:string)=>api.get<{forecastDatasets:unknown[]}>(`${base(locationId)}/datasets`).then(r=>r.data),
 runs:(locationId:string)=>api.get<{forecastRuns:ForecastRun[]}>(`${base(locationId)}/runs`).then(r=>r.data),
 createRun:(locationId:string,input:{policyId:string;idempotencyKey:string;cutoffAt?:string})=>api.post<{forecastJob:unknown;forecastDataset:unknown}>(`${base(locationId)}/runs`,input).then(r=>r.data),
 createPurchaseOrderDraft:(locationId:string,recommendationId:string,input:{supplierItemId:string;idempotencyKey:string})=>api.post<{purchaseOrder:{id:string;orderNumber:string;status:string};lines:unknown[]}>(`${base(locationId)}/reorder-recommendations/${recommendationId}/purchase-order-draft`,input).then(r=>r.data),
 points:(locationId:string,runId:string)=>api.get<{forecastPoints:ForecastPoint[]}>(`${base(locationId)}/runs/${runId}/points`).then(r=>r.data),
 maturity:(locationId:string,runId:string)=>api.get<{dataMaturity:ForecastMaturity[]}>(`${base(locationId)}/runs/${runId}/maturity`).then(r=>r.data),
 recommendations:(locationId:string,runId?:string)=>api.get<{recommendations:ForecastRecommendation[]}>(`${base(locationId)}${runId?`/runs/${runId}`:''}/recommendations`).then(r=>r.data),
 coverage:(locationId:string,runId?:string)=>api.get<{coverage:ForecastCoverage[]}>(`${base(locationId)}${runId?`/runs/${runId}`:''}/coverage`).then(r=>({ coverages:r.data.coverage })),
};
