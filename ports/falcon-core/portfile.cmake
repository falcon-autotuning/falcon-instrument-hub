vcpkg_from_github(
    OUT_SOURCE_PATH SOURCE_PATH
    REPO falcon-autotuning/falcon-core
    REF v${VERSION}
    SHA512 f40409b38dcbb159d893d863874680f48259b5db35067bf193aa29df7ef58e7c67c6ec55bb41b56a1c4998e902d55ea0fccea77c85168b1b20d7f799518f0107
)
set(BUILD_C_API OFF)
if("c-api" IN_LIST FEATURES)
  set(BUILD_C_API ON)
endif()

vcpkg_cmake_configure(
    SOURCE_PATH "${SOURCE_PATH}"
    OPTIONS
        -DFALCON_CORE_BUILD_C_API=${BUILD_C_API}
)

vcpkg_cmake_install()
vcpkg_cmake_config_fixup()

file(INSTALL "${SOURCE_PATH}/LICENSE.txt"
     DESTINATION "${CURRENT_PACKAGES_DIR}/share/${PORT}"
     RENAME copyright)

vcpkg_copy_pdbs()
