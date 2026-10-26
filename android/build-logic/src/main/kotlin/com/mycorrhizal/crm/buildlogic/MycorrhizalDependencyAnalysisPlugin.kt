package com.mycorrhizal.crm.buildlogic

import org.gradle.api.Plugin
import org.gradle.api.Project

/**
 * Issue #1908: applies the dependency-analysis plugin from build-logic's
 * classpath, where AGP and the Kotlin plugins are already loaded. The plugin
 * needs that visibility to analyze Android modules; applying it from the
 * settings script would require AGP in the settings classloader and conflict
 * with the versioned plugin aliases here.
 *
 * The Android convention plugins apply the same id to every library/app
 * module, and the root build applies this plugin for the aggregate
 * `buildHealth` task.
 */
class MycorrhizalDependencyAnalysisPlugin : Plugin<Project> {
    override fun apply(target: Project) {
        target.pluginManager.apply("com.autonomousapps.dependency-analysis")
    }
}
