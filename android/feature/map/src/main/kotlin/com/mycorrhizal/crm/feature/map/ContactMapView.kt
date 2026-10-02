package com.mycorrhizal.crm.feature.map

import android.content.Context
import android.os.Bundle
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberUpdatedState
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.viewinterop.AndroidView
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import org.maplibre.android.MapLibre
import org.maplibre.android.camera.CameraUpdateFactory
import org.maplibre.android.geometry.LatLng
import org.maplibre.android.geometry.LatLngBounds
import org.maplibre.android.maps.MapLibreMap
import org.maplibre.android.maps.MapView
import org.maplibre.android.maps.Style
import org.maplibre.android.style.expressions.Expression
import org.maplibre.android.style.layers.CircleLayer
import org.maplibre.android.style.layers.PropertyFactory
import org.maplibre.android.style.sources.GeoJsonSource
import org.maplibre.geojson.Feature
import org.maplibre.geojson.FeatureCollection
import org.maplibre.geojson.Point

private const val SOURCE_ID = "contacts"
private const val LAYER_ID = "contacts-layer"
private const val KEY_PROPERTY = "key"
private const val FRAME_PADDING_PX = 120
private const val SINGLE_POINT_ZOOM = 10.0
private const val CIRCLE_RADIUS = 9f
private const val SELECTED_CIRCLE_RADIUS = 13f

/**
 * The MapLibre canvas (ADR 0031, issue #1287): the instance's style URL, one
 * circle per plotted address, tap-to-select. Everything MapLibre-specific lives
 * here so the rest of the screen is plain Compose that tests exercise without
 * the native renderer; [MapScreen] takes this as a replaceable parameter.
 */
@Composable
fun ContactMapView(
    styleUrl: String,
    points: List<MapPoint>,
    selectedKey: String?,
    onSelect: (String?) -> Unit,
    modifier: Modifier = Modifier,
) {
    val context = LocalContext.current
    val mapView = remember { createMapView(context) }
    val currentPoints = rememberUpdatedState(points)
    val currentSelected = rememberUpdatedState(selectedKey)
    val currentOnSelect = rememberUpdatedState(onSelect)
    // The loaded map and whether its style finished; set from the async callbacks.
    val holder = remember { MapHolder() }

    val lifecycle = LocalLifecycleOwner.current.lifecycle
    DisposableEffect(lifecycle, mapView) {
        val observer = LifecycleEventObserver { _, event ->
            when (event) {
                Lifecycle.Event.ON_CREATE -> mapView.onCreate(Bundle())
                Lifecycle.Event.ON_START -> mapView.onStart()
                Lifecycle.Event.ON_RESUME -> mapView.onResume()
                Lifecycle.Event.ON_PAUSE -> mapView.onPause()
                Lifecycle.Event.ON_STOP -> mapView.onStop()
                Lifecycle.Event.ON_DESTROY -> mapView.onDestroy()
                else -> Unit
            }
        }
        lifecycle.addObserver(observer)
        // The view is created after the lifecycle may already be past CREATED
        // (it is built during composition), so replay the missed events.
        if (lifecycle.currentState.isAtLeast(Lifecycle.State.CREATED)) mapView.onCreate(Bundle())
        if (lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) mapView.onStart()
        if (lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) mapView.onResume()
        onDispose {
            lifecycle.removeObserver(observer)
            if (lifecycle.currentState.isAtLeast(Lifecycle.State.RESUMED)) mapView.onPause()
            if (lifecycle.currentState.isAtLeast(Lifecycle.State.STARTED)) mapView.onStop()
            mapView.onDestroy()
        }
    }

    DisposableEffect(styleUrl, mapView) {
        holder.styleLoaded = false
        mapView.getMapAsync { map ->
            holder.map = map
            map.setStyle(Style.Builder().fromUri(styleUrl)) { style ->
                style.addSource(GeoJsonSource(SOURCE_ID, FeatureCollection.fromFeatures(emptyList())))
                style.addLayer(
                    CircleLayer(LAYER_ID, SOURCE_ID).withProperties(
                        PropertyFactory.circleColor("#3b5340"),
                        PropertyFactory.circleStrokeColor("#ffffff"),
                        PropertyFactory.circleStrokeWidth(2f),
                        PropertyFactory.circleRadius(CIRCLE_RADIUS),
                    ),
                )
                holder.styleLoaded = true
                holder.framedFor = null
                render(map, style, currentPoints.value, currentSelected.value, holder)
            }
            map.addOnMapClickListener { latLng ->
                val screen = map.projection.toScreenLocation(latLng)
                val hit = map.queryRenderedFeatures(screen, LAYER_ID).firstOrNull()
                currentOnSelect.value(hit?.getStringProperty(KEY_PROPERTY))
                hit != null
            }
        }
        onDispose { holder.styleLoaded = false }
    }

    AndroidView(
        factory = { mapView },
        modifier = modifier,
        update = {
            val map = holder.map
            val style = map?.style
            if (map != null && style != null && holder.styleLoaded) {
                render(map, style, points, selectedKey, holder)
            }
        },
    )
}

private class MapHolder {
    var map: MapLibreMap? = null
    var styleLoaded: Boolean = false
    /** The point set the camera was last framed for, so a selection change does not re-frame. */
    var framedFor: List<String>? = null
}

private fun createMapView(context: Context): MapView {
    // Idempotent; must run before the first MapView is constructed.
    MapLibre.getInstance(context)
    return MapView(context)
}

private fun render(
    map: MapLibreMap,
    style: Style,
    points: List<MapPoint>,
    selectedKey: String?,
    holder: MapHolder,
) {
    val features = points.map { p ->
        Feature.fromGeometry(Point.fromLngLat(p.position.longitude, p.position.latitude)).also {
            it.addStringProperty(KEY_PROPERTY, p.key)
        }
    }
    style.getSourceAs<GeoJsonSource>(SOURCE_ID)?.setGeoJson(FeatureCollection.fromFeatures(features))
    style.getLayerAs<CircleLayer>(LAYER_ID)?.setProperties(
        PropertyFactory.circleRadius(
            Expression.switchCase(
                Expression.eq(
                    Expression.get(KEY_PROPERTY),
                    Expression.literal(selectedKey.orEmpty()),
                ),
                Expression.literal(SELECTED_CIRCLE_RADIUS),
                Expression.literal(CIRCLE_RADIUS),
            ),
        ),
    )

    val keys = points.map { it.key }
    if (holder.framedFor == keys) return
    holder.framedFor = keys
    when (val viewport = viewportFor(points)) {
        MapViewport.World -> Unit
        is MapViewport.Single -> map.moveCamera(
            CameraUpdateFactory.newLatLngZoom(
                LatLng(viewport.position.latitude, viewport.position.longitude),
                SINGLE_POINT_ZOOM,
            ),
        )
        is MapViewport.Bounds -> map.moveCamera(
            CameraUpdateFactory.newLatLngBounds(
                LatLngBounds.Builder()
                    .include(LatLng(viewport.southWest.latitude, viewport.southWest.longitude))
                    .include(LatLng(viewport.northEast.latitude, viewport.northEast.longitude))
                    .build(),
                FRAME_PADDING_PX,
            ),
        )
    }
}
