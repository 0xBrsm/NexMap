cmake_minimum_required(VERSION 3.10)
project(navcheck CXX)

set(CMAKE_CXX_STANDARD 11)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
if(NOT CMAKE_BUILD_TYPE)
  set(CMAKE_BUILD_TYPE Release)
endif()

# Recast/Detour from the vendored upstream clone (vendor/recastnavigation).
# Build only the libraries we link — the demo/tests/examples need SDL/OpenGL.
set(RECASTNAVIGATION_DEMO OFF CACHE BOOL "" FORCE)
set(RECASTNAVIGATION_TESTS OFF CACHE BOOL "" FORCE)
set(RECASTNAVIGATION_EXAMPLES OFF CACHE BOOL "" FORCE)

add_subdirectory(${CMAKE_CURRENT_SOURCE_DIR}/../../vendor/recastnavigation
                 ${CMAKE_BINARY_DIR}/recastnavigation EXCLUDE_FROM_ALL)

add_executable(navcheck main.cpp nav_mesh.cpp nav_hull.cpp qworld.cpp nav_links.cpp)
target_compile_options(navcheck PRIVATE -fno-rtti)  # match Recast (built without RTTI)
target_link_libraries(navcheck Recast Detour DetourCrowd)
